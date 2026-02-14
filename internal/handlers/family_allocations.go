package handlers

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
)

// RoomsInventoryItem represents a single room type in inventory
type RoomsInventoryItem struct {
	RoomOfferID  string `json:"room_offer_id"`
	RoomName     string `json:"room_name"`
	Available    int    `json:"available"`
	MaxCapacity  int    `json:"max_capacity"`
	PricePerRoom int    `json:"price_per_room"`
}

// AllocateFamilyRequest represents the request body for family allocation
type AllocateFamilyRequest struct {
	EventID     string `json:"event_id" validate:"required"`
	FamilyID    string `json:"family_id" validate:"required"`
	RoomOfferID string `json:"room_offer_id" validate:"required"`
	// Note: assigned_by removed - derived from authenticated user
}

// AllocateFamilyHandler allocates a family to a room with concurrency safety
func AllocateFamilyHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract authenticated user from context
		userID := c.Locals("userID")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		userUUID, ok := userID.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user ID type",
			})
		}

		// Get user role from context
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		role, ok := userRole.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user role type",
			})
		}

		// Derive assigned_mode from authenticated user role (server-side only)
		var assignedMode string
		if role == "agent" {
			assignedMode = "agent_manual"
		} else if role == "head_guest" {
			assignedMode = "head_guest_manual"
		} else {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		var req AllocateFamilyRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate UUIDs
		eventID, err := uuid.Parse(req.EventID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid event_id",
			})
		}

		familyID, err := uuid.Parse(req.FamilyID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid family_id",
			})
		}

		// BEGIN TRANSACTION with row-level locking
		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// Step 1: Lock the event row using SELECT FOR UPDATE
		var event models.Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", eventID).
			First(&event).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Event not found",
			})
		}

		// Step 2: Enforce strict ownership validation
		if role == "agent" {
			if userUUID != event.AgentID {
				tx.Rollback()
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		} else if role == "head_guest" {
			if userUUID != event.HeadGuestID {
				tx.Rollback()
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		}

		// Step 3: Check if event allows allocation
		if event.Status == "rooms_finalized" {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Cannot allocate rooms. Event status is 'rooms_finalized'",
			})
		}

		if event.Status != "allocating" {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": fmt.Sprintf("Cannot allocate rooms. Event status is '%s'", event.Status),
			})
		}

		// Step 4: Get all guests in the family
		var familyGuests []models.Guest
		if err := tx.Where("event_id = ? AND family_id = ?", eventID, familyID).
			Find(&familyGuests).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch family guests",
			})
		}

		if len(familyGuests) == 0 {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No guests found for this family",
			})
		}

		familySize := len(familyGuests)

		// Step 5: Get room offer and check capacity
		var roomOffer models.RoomOffer
		if err := tx.Where("id = ?", req.RoomOfferID).First(&roomOffer).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Room offer not found",
			})
		}

		if familySize > roomOffer.MaxCapacity {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Family size (%d) exceeds room capacity (%d)", familySize, roomOffer.MaxCapacity),
			})
		}

		// Step 6: Parse and check inventory
		var inventory []RoomsInventoryItem
		if err := json.Unmarshal(event.RoomsInventory, &inventory); err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to parse inventory",
			})
		}

		// Find the room in inventory
		roomIndex := -1
		for i, item := range inventory {
			if item.RoomOfferID == req.RoomOfferID {
				roomIndex = i
				break
			}
		}

		if roomIndex == -1 {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Room not found in event inventory",
			})
		}

		if inventory[roomIndex].Available <= 0 {
			tx.Rollback()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "No rooms available for this type",
			})
		}

		// Step 7: Check if any family member is already allocated
		var existingAllocations []models.GuestAllocation
		guestIDs := make([]uuid.UUID, len(familyGuests))
		for i, guest := range familyGuests {
			guestIDs[i] = guest.ID
		}

		if err := tx.Where("event_id = ? AND guest_id IN ?", eventID, guestIDs).
			Find(&existingAllocations).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to check existing allocations",
			})
		}

		if len(existingAllocations) > 0 {
			tx.Rollback()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "One or more family members are already allocated",
			})
		}

		// Step 8: Create allocations for all family members
		allocations := make([]models.GuestAllocation, len(familyGuests))
		for i, guest := range familyGuests {
			allocations[i] = models.GuestAllocation{
				ID:           uuid.New(),
				EventID:      eventID,
				GuestID:      guest.ID,
				RoomOfferID:  &req.RoomOfferID,
				LockedPrice:  roomOffer.TotalFare,
				Status:       "allocated",
				AssignedMode: assignedMode, // Server-derived, not from client
			}
		}

		if err := tx.Create(&allocations).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create allocations",
			})
		}

		// Step 9: Decrement inventory
		inventory[roomIndex].Available--

		updatedInventoryJSON, err := json.Marshal(inventory)
		if err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update inventory",
			})
		}

		if err := tx.Model(&event).Update("rooms_inventory", datatypes.JSON(updatedInventoryJSON)).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save inventory",
			})
		}

		// COMMIT transaction
		if err := tx.Commit().Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Transaction commit failed",
			})
		}

		log.Printf("✅ Family %s allocated to room %s (Event: %s)", familyID, req.RoomOfferID, eventID)

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message":       "Family allocated successfully",
			"family_id":     familyID,
			"room_offer_id": req.RoomOfferID,
			"family_size":   familySize,
			"allocations":   allocations,
		})
	}
}

// FinalizeRoomsHandler finalizes room allocations for an event
func FinalizeRoomsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract authenticated user from context
		userID := c.Locals("userID")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		userUUID, ok := userID.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user ID type",
			})
		}

		// Get user role from context
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		role, ok := userRole.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user role type",
			})
		}

		// Validate role
		if role != "agent" && role != "head_guest" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		eventID := c.Params("eventId")

		eventUUID, err := uuid.Parse(eventID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid event_id",
			})
		}

		var event models.Event
		if err := db.Where("id = ?", eventUUID).First(&event).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Event not found",
			})
		}

		// Enforce strict ownership validation
		if role == "agent" {
			if userUUID != event.AgentID {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		} else if role == "head_guest" {
			if userUUID != event.HeadGuestID {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		}

		if event.Status == "rooms_finalized" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Rooms are already finalized",
			})
		}

		// Update status to rooms_finalized
		if err := db.Model(&event).Update("status", "rooms_finalized").Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to finalize rooms",
			})
		}

		log.Printf("✅ Event %s rooms finalized", eventID)

		return c.JSON(fiber.Map{
			"message":  "Rooms finalized successfully",
			"event_id": eventID,
			"status":   "rooms_finalized",
		})
	}
}

// ReopenAllocationHandler reopens allocation for an event (agent only)
func ReopenAllocationHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract authenticated user from context
		userID := c.Locals("userID")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		userUUID, ok := userID.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user ID type",
			})
		}

		// Get user role from context
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		role, ok := userRole.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user role type",
			})
		}

		// Reopen is agent-only operation
		if role != "agent" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		eventID := c.Params("eventId")

		eventUUID, err := uuid.Parse(eventID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid event_id",
			})
		}

		var event models.Event
		if err := db.Where("id = ?", eventUUID).First(&event).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Event not found",
			})
		}

		// Enforce strict ownership validation (agent only)
		if userUUID != event.AgentID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		if event.Status != "rooms_finalized" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Event is not finalized",
			})
		}

		// Reopen allocation
		if err := db.Model(&event).Update("status", "allocating").Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to reopen allocation",
			})
		}

		log.Printf("✅ Event %s allocation reopened", eventID)

		return c.JSON(fiber.Map{
			"message":  "Allocation reopened successfully",
			"event_id": eventID,
			"status":   "allocating",
		})
	}
}

// CheckInFamilyHandler checks in all members of a family
func CheckInFamilyHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract authenticated user from context
		userID := c.Locals("userID")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		userUUID, ok := userID.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user ID type",
			})
		}

		// Get user role from context
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		role, ok := userRole.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Invalid user role type",
			})
		}

		// Validate role
		if role != "agent" && role != "head_guest" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		eventID := c.Params("eventId")
		familyID := c.Params("familyId")

		eventUUID, err := uuid.Parse(eventID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid event_id",
			})
		}

		familyUUID, err := uuid.Parse(familyID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid family_id",
			})
		}

		// Fetch event to validate ownership
		var event models.Event
		if err := db.Where("id = ?", eventUUID).First(&event).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Event not found",
			})
		}

		// Enforce strict ownership validation
		if role == "agent" {
			if userUUID != event.AgentID {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		} else if role == "head_guest" {
			if userUUID != event.HeadGuestID {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"error": "Not authorized for this event",
				})
			}
		}

		// Get all guests in the family
		var familyGuests []models.Guest
		if err := db.Where("event_id = ? AND family_id = ?", eventUUID, familyUUID).
			Find(&familyGuests).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch family guests",
			})
		}

		if len(familyGuests) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No guests found for this family",
			})
		}

		guestIDs := make([]uuid.UUID, len(familyGuests))
		for i, guest := range familyGuests {
			guestIDs[i] = guest.ID
		}

		// Update all allocations to checked_in
		result := db.Model(&models.GuestAllocation{}).
			Where("event_id = ? AND guest_id IN ?", eventUUID, guestIDs).
			Update("status", "checked_in")

		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to check in family",
			})
		}

		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "No allocations found for this family",
			})
		}

		log.Printf("✅ Family %s checked in (Event: %s)", familyID, eventID)

		return c.JSON(fiber.Map{
			"message":         "Family checked in successfully",
			"family_id":       familyID,
			"guests_affected": result.RowsAffected,
		})
	}
}
