package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
)

// Helper to generate a random password
func generateRandomPassword(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:n], nil
}

func (m *Repository) GetEvents(c *fiber.Ctx) error {
	userID := c.Locals("userID")
	if userID == nil {
		return utils.ErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	// userID is stored as uuid.UUID in context by middleware
	agentID, ok := userID.(uuid.UUID)
	if !ok {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Invalid User ID type")
	}

	var events []models.Event
	if err := m.DB.Where("agent_id = ?", agentID).Find(&events).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch events")
	}

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Events Fetched Successfully",
		"events":  events,
	})
}

func (m *Repository) GetEvent(c *fiber.Ctx) error {
	id := c.Params("id")

	// Validate UUID
	if _, err := uuid.Parse(id); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}

	var event models.Event
	if err := m.DB.Where("id = ?", id).First(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Event Fetched",
		"event":   event,
	})
}

type CreateEventRequest struct {
	Name           string         `json:"name"` // Added Name field
	HotelID        string         `json:"hotelId"`
	Location       string         `json:"location"`
	StartDate      string         `json:"startDate"`
	EndDate        string         `json:"endDate"`
	RoomsInventory map[string]int `json:"roomsInventory"`
}

func (m *Repository) CreateEvent(c *fiber.Ctx) error {
	userID := c.Locals("userID")
	if userID == nil {
		return utils.ErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	agentID, ok := userID.(uuid.UUID)
	if !ok {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Invalid User ID type")
	}

	var req CreateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// Basic Validation
	if req.Location == "" || req.StartDate == "" || req.EndDate == "" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Missing required fields")
	}

	// Try parsing dates. Frontend sends YYYY-MM-DD usually for date inputs.
	layout := "2006-01-02"
	startDate, err := time.Parse(layout, req.StartDate)
	if err != nil {
		// Try RFC3339 as fallback
		startDate, err = time.Parse(time.RFC3339, req.StartDate)
		if err != nil {
			return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid StartDate format. Expected YYYY-MM-DD")
		}
	}

	endDate, err := time.Parse(layout, req.EndDate)
	if err != nil {
		endDate, err = time.Parse(time.RFC3339, req.EndDate)
		if err != nil {
			return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid EndDate format. Expected YYYY-MM-DD")
		}
	}

	// JSONB handling for RoomsInventory
	roomsJSON, _ := json.Marshal(req.RoomsInventory)

	event := models.Event{
		ID:             uuid.New(),
		AgentID:        agentID,
		HotelID:        req.HotelID,
		Location:       req.Location,
		StartDate:      startDate,
		EndDate:        endDate,
		Status:         "draft",
		RoomsInventory: datatypes.JSON(roomsJSON),
	}

	if err := m.DB.Create(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to create event")
	}

	return utils.SuccessResponse(c, fiber.StatusCreated, fiber.Map{
		"message": "Event Created Successfully",
		"event":   event,
	})
}

func (m *Repository) GetMetrics(c *fiber.Ctx) error {
	// TODO: Get metrics from store
	// metrics, err := m.DB.GetMetrics()
	// if err != nil {
	//     return utils.InternalErrorResponse(c, "Failed to fetch metrics")
	// }

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Get Metrics Endpoint",
		"metrics": []interface{}{},
	})
}

func (m *Repository) GetEventVenues(c *fiber.Ctx) error {
	// Get event ID from path parameter
	id := c.Params("id")

	// TODO: Get venues for event
	// venues, err := m.DB.GetVenuesByEventID(id)
	// if err != nil {
	//     return utils.InternalErrorResponse(c, "Failed to fetch venues")
	// }

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Get Event Venues Endpoint",
		"eventId": id,
		"venues":  []interface{}{},
	})
}

func (m *Repository) GetEventAllocations(c *fiber.Ctx) error {
	// Get event ID from path parameter
	id := c.Params("id")

	// TODO: Get allocations for event
	// allocations, err := m.DB.GetAllocationsByEventID(id)
	// if err != nil {
	//     return utils.InternalErrorResponse(c, "Failed to fetch allocations")
	// }

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Get Event Allocations Endpoint",
		"eventId": id,
	})
}

func (m *Repository) UpdateEvent(c *fiber.Ctx) error {
	// Get event ID from path parameter
	id := c.Params("id")

	// TODO: Parse request body and update event
	// var event models.Event
	// if err := c.BodyParser(&event); err != nil {
	//     return utils.ValidationErrorResponse(c, "Invalid request body")
	// }

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Update Event Endpoint",
		"id":      id,
	})
}

func (m *Repository) DeleteEvent(c *fiber.Ctx) error {
	// Get event ID from path parameter
	id := c.Params("id")

	// TODO: Delete event
	// if err := m.DB.DeleteEvent(id); err != nil {
	//     return utils.InternalErrorResponse(c, "Failed to delete event")
	// }

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Delete Event Endpoint",
		"id":      id,
	})
}

type AssignHeadGuestRequest struct {
	ClerkID string `json:"clerkId"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
}

func (m *Repository) AssignHeadGuest(c *fiber.Ctx) error {
	id := c.Params("id")
	var req AssignHeadGuestRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Email == "" { // Removed ClerkID check as it's no longer used for user creation
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Missing required fields")
	}

	// Start a transaction
	tx := m.DB.Begin()
	if tx.Error != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to start transaction")
	}

	// 1. Create or Find User
	// Create the Head Guest User account
	// TODO: Send email to head guest to set their password.
	// For now, setting a default password or handling this logic is required since PasswordHash is Not Null.
	// We'll use a placeholder hash for "ChangeMe123!"
	// defaultHash := "$2a$10$3QxDjD1ylg.6T4x.5.6.7.8.9.0.1.2.3.4.5.6.7.8.9.0.1.2" // Example hash or generate real one
	// Actually, let's use a dummy hash or generated one to avoid empty string error.

	// Better: Generate a random password and hash it, maybe print it or just store it.
	// simpler: just put a valid bcrypt hash to satisfy constraint.

	var user models.User
	var tempPassword string

	// Check if a user with this email already exists
	if err := tx.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// If user does not exist, create a new one
		if err.Error() == "record not found" {
			// Generate valid random password
			generatedPwd, err := generateRandomPassword(12)
			if err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to generate password")
			}
			tempPassword = generatedPwd

			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
			if err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to hash password")
			}

			user = models.User{
				ID:           uuid.New(), // Explicitly generate UUID
				Email:        req.Email,
				Role:         "head_guest",
				Name:         req.Name,
				Phone:        req.Phone,
				PasswordHash: string(hashedPassword), // Placeholder
			}

			if err := tx.Create(&user).Error; err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to create head guest user: "+err.Error())
			}
		} else {
			tx.Rollback()
			return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to query user: "+err.Error())
		}
	} else {
		// If user exists, update their details if necessary and ensure role is head_guest
		if user.Role != "head_guest" {
			user.Role = "head_guest"
			if err := tx.Model(&user).Update("role", "head_guest").Error; err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update existing user's role: "+err.Error())
			}
		}
		// Update other fields if provided in the request
		if req.Name != "" && user.Name != req.Name {
			if err := tx.Model(&user).Update("name", req.Name).Error; err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update existing user's name: "+err.Error())
			}
		}
		if req.Phone != "" && user.Phone != req.Phone {
			if err := tx.Model(&user).Update("phone", req.Phone).Error; err != nil {
				tx.Rollback()
				return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update existing user's phone: "+err.Error())
			}
		}
	}

	// 2. Update Event
	// We cast string id to UUID or let GORM handle it? GORM usually handles string -> UUID if model is defined correctly.
	// However, it's safer to check if event exists first.
	var event models.Event
	if err := tx.Where("id = ?", id).First(&event).Error; err != nil {
		tx.Rollback()
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}

	if err := tx.Model(&event).Update("head_guest_id", user.ID).Error; err != nil {
		tx.Rollback()
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update event")
	}

	// 3. Create Guest Record (optional, but requested in previous conversations)
	// We will just create a basic guest entry
	guest := models.Guest{
		EventID: event.ID,
		Name:    req.Name,
		Email:   req.Email,
		Phone:   req.Phone,
		Type:    "Adult",
	}
	if err := tx.Create(&guest).Error; err != nil {
		tx.Rollback()
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to create guest record")
	}

	if err := tx.Commit().Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to commit transaction")
	}

	response := fiber.Map{
		"message": "Head Guest Assigned Successfully",
		"user":    user,
	}
	if tempPassword != "" {
		response["tempPassword"] = tempPassword
	}

	return utils.SuccessResponse(c, fiber.StatusOK, response)
}
