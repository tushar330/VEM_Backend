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

// Response structures for GetAllocations
type GuestResponse struct {
	GuestID   string `json:"guest_id"`
	GuestName string `json:"guest_name"`
}

type FamilyAllocationResponse struct {
	FamilyID     string          `json:"family_id"`
	AllocationID string          `json:"allocation_id"`
	RoomOfferID  *string         `json:"room_offer_id"`
	RoomName     string          `json:"room_name"`
	Guests       []GuestResponse `json:"guests"`
}

type EventAllocationsResponse struct {
	EventID        string                     `json:"event_id"`
	Status         string                     `json:"status"`
	RoomsInventory []RoomsInventoryItem       `json:"rooms_inventory"`
	Allocations    []FamilyAllocationResponse `json:"allocations"`
}

// RoomsInventoryItem represents a single room type in inventory
type RoomsInventoryItem struct {
	RoomOfferID  string `json:"room_offer_id"`
	RoomName     string `json:"room_name"`
	Available    int    `json:"available"`
	MaxCapacity  int    `json:"max_capacity"`
	PricePerRoom int    `json:"price_per_room"`
}

// GetEventAllocationsHandler retrieves all allocations for an event, grouped by family
func GetEventAllocationsHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// support both :eventId (new) and :id (legacy/standard)
		eventID := c.Params("eventId")
		if eventID == "" {
			eventID = c.Params("id")
		}

		log.Printf("🔍 GetEventAllocations: Requested Event ID: %s", eventID)

		eventUUID, err := uuid.Parse(eventID)
		if err != nil {
			log.Printf("❌ Invalid UUID format: %s", eventID)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event ID"})
		}

		// 1. Fetch Event for Status and Inventory
		var event models.Event
		if err := db.First(&event, "id = ?", eventUUID).Error; err != nil {
			log.Printf("❌ Event not found in DB: %s (Error: %v)", eventUUID, err)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
		}
		log.Printf("✅ Event found: %s (Status: %s)", event.ID, event.Status)

		// 2. Parse Inventory
		var inventory []RoomsInventoryItem
		if len(event.RoomsInventory) > 0 {
			if err := json.Unmarshal(event.RoomsInventory, &inventory); err != nil {
				// Log error but don't fail, return empty inventory
				log.Printf("Error unmarshalling inventory: %v", err)
				inventory = []RoomsInventoryItem{}
			}
		} else {
			inventory = []RoomsInventoryItem{}
		}

		// 3. Fetch Allocations with Guests and RoomOffers
		var allocations []models.GuestAllocation
		if err := db.Preload("Guest").Preload("RoomOffer").
			Where("event_id = ?", eventUUID).
			Find(&allocations).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch allocations"})
		}

		// 4. Group by Family
		familyMap := make(map[string]*FamilyAllocationResponse)

		for _, alloc := range allocations {
			familyID := alloc.Guest.FamilyID.String()

			if _, exists := familyMap[familyID]; !exists {
				roomName := ""
				if alloc.RoomOfferID != nil {
					roomName = alloc.RoomOffer.Name
				}

				familyMap[familyID] = &FamilyAllocationResponse{
					FamilyID:     familyID,
					AllocationID: alloc.ID.String(), // Pick one allocation ID
					RoomOfferID:  alloc.RoomOfferID,
					RoomName:     roomName,
					Guests:       []GuestResponse{},
				}
			}

			familyMap[familyID].Guests = append(familyMap[familyID].Guests, GuestResponse{
				GuestID:   alloc.GuestID.String(),
				GuestName: alloc.Guest.Name,
			})
		}

		// Convert map to slice
		groupedAllocations := make([]FamilyAllocationResponse, 0, len(familyMap))
		for _, fam := range familyMap {
			groupedAllocations = append(groupedAllocations, *fam)
		}

		return c.JSON(EventAllocationsResponse{
			EventID:        event.ID.String(),
			Status:         event.Status,
			RoomsInventory: inventory,
			Allocations:    groupedAllocations,
		})
	}
}

// ... existing handlers ...

// Update AllocateFamilyHandler return to use snake_case keys (Partial replacement shown for where it returns)
// Note: I will need to verify if I can replace just the return statement or if I should replace the whole function to be safe.
// Given the file size, I will append the NEW handler at the end (if not replacing), but I need to UPDATE existing handlers.
// Strategy: I will replace the return statements in `AllocateFamilyHandler` and `UpdateAllocationHandler`.

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
			"message":         "Family allocated successfully",
			"family_id":       familyID,
			"room_offer_id":   req.RoomOfferID,
			"guests_affected": familySize,
			"assigned_mode":   assignedMode,
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

// UpdateAllocationHandler handles updating a family allocation to a new room offer
func UpdateAllocationHandler(db *gorm.DB) fiber.Handler {
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

		// Derive assigned_mode from authenticated user role
		var assignedMode string
		if role == "agent" {
			assignedMode = "agent_manual_update"
		} else if role == "head_guest" {
			assignedMode = "guest_manual_update"
		} else {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Not authorized for this event",
			})
		}

		allocationID := c.Params("id")
		allocUUID, err := uuid.Parse(allocationID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid allocation ID",
			})
		}

		// Parse request body for new room offer
		var req struct {
			RoomOfferID string `json:"room_offer_id" validate:"required"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if req.RoomOfferID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "room_offer_id is required",
			})
		}

		// BEGIN TRANSACTION
		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// Step 1: Find the initial allocation to get EventID and Family details
		// We lock this row to prevent concurrent updates to the same allocation
		var targetAllocation models.GuestAllocation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Guest").
			First(&targetAllocation, "id = ?", allocUUID).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Allocation not found",
			})
		}

		eventID := targetAllocation.EventID

		// Optimization: Check if new room is same as old room
		if targetAllocation.RoomOfferID != nil && *targetAllocation.RoomOfferID == req.RoomOfferID {
			tx.Rollback()
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"message": "Allocation updated successfully (No change)",
			})
		}

		// Step 2: Lock the Event row (for inventory safety)
		var event models.Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", eventID).
			First(&event).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Event not found",
			})
		}

		// Step 3: Enforce strict ownership validation
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

		// Step 4: Check Event Status
		if event.Status != "allocating" {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": fmt.Sprintf("Cannot update allocation. Event status is '%s'", event.Status),
			})
		}

		// Step 5: Get all allocations for this family/group to ensure we move everyone
		// We use the Guest's FamilyID from the target allocation
		familyID := targetAllocation.Guest.FamilyID

		var familyAllocations []models.GuestAllocation
		// Simpler: Find all guests for this family first, then find allocations
		var familyGuests []models.Guest
		if err := tx.Where("event_id = ? AND family_id = ?", eventID, familyID).Find(&familyGuests).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch family guests",
			})
		}

		guestIDs := make([]uuid.UUID, len(familyGuests))
		for i, g := range familyGuests {
			guestIDs[i] = g.ID
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("event_id = ? AND guest_id IN ?", eventID, guestIDs).
			Find(&familyAllocations).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to fetch family allocations",
			})
		}

		// Step 6: Validate New Room Capacity
		familySize := len(familyGuests)
		var newRoomOffer models.RoomOffer
		if err := tx.Where("id = ?", req.RoomOfferID).First(&newRoomOffer).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "New room offer not found",
			})
		}

		if familySize > newRoomOffer.MaxCapacity {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Family size (%d) exceeds new room capacity (%d)", familySize, newRoomOffer.MaxCapacity),
			})
		}

		// Step 7: Inventory Swap Logic
		var inventory []RoomsInventoryItem
		if err := json.Unmarshal(event.RoomsInventory, &inventory); err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to parse inventory",
			})
		}

		// Locate Old Room and New Room indices
		oldRoomID := targetAllocation.RoomOfferID // This might be nil if previously unallocated, but here we assume allocated
		oldRoomIndex := -1
		newRoomIndex := -1

		for i, item := range inventory {
			if oldRoomID != nil && item.RoomOfferID == *oldRoomID {
				oldRoomIndex = i
			}
			if item.RoomOfferID == req.RoomOfferID {
				newRoomIndex = i
			}
		}

		if newRoomIndex == -1 {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "New room not found in inventory",
			})
		}

		// Increment Old Room Availability (if it existed)
		if oldRoomIndex != -1 {
			inventory[oldRoomIndex].Available++
		}

		// Decrement New Room Availability
		if inventory[newRoomIndex].Available <= 0 {
			tx.Rollback()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "New room is no longer available",
			})
		}
		inventory[newRoomIndex].Available--

		// Save Inventory
		updatedInventoryJSON, err := json.Marshal(inventory)
		if err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update inventory JSON",
			})
		}

		if err := tx.Model(&event).Update("rooms_inventory", datatypes.JSON(updatedInventoryJSON)).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save inventory to DB",
			})
		}

		// Step 8: Update Allocations
		// Updates all allocations for the family to the new room
		updates := map[string]interface{}{
			"room_offer_id": req.RoomOfferID,
			"locked_price":  newRoomOffer.TotalFare,
			"assigned_mode": assignedMode,
		}

		if err := tx.Model(&models.GuestAllocation{}).
			Where("event_id = ? AND guest_id IN ?", eventID, guestIDs).
			Updates(updates).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update allocations",
			})
		}

		// Step 9: Commit
		if err := tx.Commit().Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Transaction commit failed",
			})
		}

		log.Printf("✅ Family %s moved to room %s (Event: %s)", familyID, req.RoomOfferID, eventID)

		return c.JSON(fiber.Map{
			"message":           "Allocation updated successfully",
			"allocation_id":     allocationID,
			"family_id":         familyID.String(),
			"old_room_offer_id": oldRoomID,
			"new_room_offer_id": req.RoomOfferID,
			"assigned_mode":     assignedMode,
			"guests_affected":   len(guestIDs),
		})
	}
}

// AutoAllocateHandler automatically assigns rooms to unallocated families
func AutoAllocateHandler(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract authenticated user
		userID := c.Locals("userID")
		if userID == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		userUUID, ok := userID.(uuid.UUID)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user ID type"})
		}

		// Check role
		userRole := c.Locals("role")
		if userRole == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		role, ok := userRole.(string)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid user role type"})
		}

		if role != "agent" && role != "head_guest" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized"})
		}

		eventID := c.Params("eventId")
		eventUUID, err := uuid.Parse(eventID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid event_id"})
		}

		// BEGIN TRANSACTION
		tx := db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// 1. Fetch Event with Lock
		var event models.Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", eventUUID).First(&event).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Event not found"})
		}

		// 2. Ownership Check
		if role == "agent" && userUUID != event.AgentID {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized"})
		} else if role == "head_guest" && userUUID != event.HeadGuestID {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Not authorized"})
		}

		// 3. Status Check
		if event.Status != "allocating" {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Event is not in 'allocating' status"})
		}

		// 4. Parse Inventory
		var inventory []RoomsInventoryItem
		if err := json.Unmarshal(event.RoomsInventory, &inventory); err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to parse inventory"})
		}

		// 5. Fetch Family Guests (Unallocated)
		// Logic: Get all guest allocations for this event.
		// Then get all guests for this event.
		// Filter out guests who are in allocations.

		var allAllocations []models.GuestAllocation
		if err := tx.Where("event_id = ?", eventUUID).Find(&allAllocations).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch allocations"})
		}

		allocatedGuestIDs := make(map[uuid.UUID]bool)
		for _, alloc := range allAllocations {
			allocatedGuestIDs[alloc.GuestID] = true
		}

		var allGuests []models.Guest
		if err := tx.Where("event_id = ?", eventUUID).Find(&allGuests).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch guests"})
		}

		var unallocatedGuests []models.Guest
		for _, guest := range allGuests {
			if !allocatedGuestIDs[guest.ID] {
				unallocatedGuests = append(unallocatedGuests, guest)
			}
		}

		if len(unallocatedGuests) == 0 {
			tx.Rollback()
			return c.JSON(fiber.Map{"message": "No unallocated guests found", "allocations_count": 0})
		}

		// Group by Family
		familyGroups := make(map[string][]models.Guest)
		for _, g := range unallocatedGuests {
			fid := g.FamilyID.String()
			familyGroups[fid] = append(familyGroups[fid], g)
		}

		totalAllocatedFamilies := 0
		totalGuestsAllocated := 0

		// 6. Allocate Loop
		for _, members := range familyGroups {
			size := len(members)

			// Find a room
			selectedRoomIndex := -1
			var selectedRoomOfferID string
			var selectedRoomPrice int

			for i, room := range inventory {
				if room.Available > 0 && room.MaxCapacity >= size {
					selectedRoomIndex = i
					selectedRoomOfferID = room.RoomOfferID
					selectedRoomPrice = room.PricePerRoom // simplistic price logic
					break                                 // Greedy
				}
			}

			if selectedRoomIndex != -1 {
				// Allocate
				inventory[selectedRoomIndex].Available--

				// Create Allocations
				allocs := make([]models.GuestAllocation, size)
				for i, m := range members {
					allocs[i] = models.GuestAllocation{
						ID:           uuid.New(),
						EventID:      eventUUID,
						GuestID:      m.ID,
						RoomOfferID:  &selectedRoomOfferID,
						LockedPrice:  float64(selectedRoomPrice),
						Status:       "allocated",
						AssignedMode: "auto_assign",
					}
				}

				if err := tx.Create(&allocs).Error; err != nil {
					log.Printf("Failed to auto-allocate family: %v", err)
					tx.Rollback()
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create allocations"})
				}

				totalAllocatedFamilies++
				totalGuestsAllocated += size
			}
		}

		// 7. Save Inventory
		newInvJSON, _ := json.Marshal(inventory)
		if err := tx.Model(&event).Update("rooms_inventory", datatypes.JSON(newInvJSON)).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update inventory"})
		}

		if err := tx.Commit().Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Commit failed"})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message":           "Auto-allocation completed",
			"allocations_count": totalAllocatedFamilies,
			"guests_count":      totalGuestsAllocated,
			"inventory_updated": true,
		})
	}
}
