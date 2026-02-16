package handlers_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestGetAllocationsTotalField(t *testing.T) {
	db := setupTestDB(t)

	// Setup specific event with inventory having total
	eventIDStr := "7e930542-4c3c-4c1a-91ea-c193116acedc"
	eventID, _ := uuid.Parse(eventIDStr)

	// Explicitly construct JSON with total
	invJSON := `[{"room_offer_id": "test_room", "room_name": "Suite", "total": 5, "available": 5, "max_capacity": 4, "price_per_room": 100}]`

	// Create Agent first to be safe
	agent := models.User{ID: uuid.New(), Name: "DebugAgent", Email: "debug@tbo.com", Role: "agent"}
	db.Create(&agent)

	event := models.Event{
		ID:             eventID,
		AgentID:        agent.ID,
		Name:           "Debug Event",
		Status:         "draft",
		RoomsInventory: datatypes.JSON(invJSON),
	}
	db.Create(&event)

	// Setup Fiber
	app := fiber.New()
	// Check signature of GetEventAllocationsHandler. It returns fiber.Handler.
	app.Get("/events/:id/allocations", handlers.GetEventAllocationsHandler(db))

	// Request
	req := httptest.NewRequest("GET", "/events/"+eventIDStr+"/allocations", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse Response
	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	// Inspect rooms_inventory
	invRaw, ok := respBody["rooms_inventory"].([]interface{})
	require.True(t, ok, "rooms_inventory should be a list")
	require.NotEmpty(t, invRaw)

	item := invRaw[0].(map[string]interface{})

	t.Logf("Inventory Item: %+v", item)

	// Check total
	_, hasTotal := item["total"]
	assert.True(t, hasTotal, "Item should have 'total' field")

	if hasTotal {
		totalVal := item["total"].(float64) // JSON numbers are floats in generic map
		assert.Equal(t, 5.0, totalVal)
	}
}
