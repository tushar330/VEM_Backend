package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/middleware"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reusing setupTestDB and seedTestData from allocation_api_test.go (implicitly available in same package)
// If they are not exported, I might need to duplicate them or modify allocation_api_test.go to export them.
// Since they are in the same package `handlers_test`, they should be available if I use the same package name.
// However, `allocation_api_test.go` uses `package handlers_test`. So it should work.

func TestInventoryTotalPersistence(t *testing.T) {
	db := setupTestDB(t)
	// clean db
	db.Exec("TRUNCATE TABLE events, guest_allocations, users RESTART IDENTITY CASCADE")

	// 1. Setup Agent
	agent := models.User{
		ID:    uuid.New(),
		Name:  "Test Agent",
		Email: "agent@test.com",
		Role:  "agent",
	}
	db.Create(&agent)
	token, _ := utils.GenerateToken(agent.ID, agent.Email, agent.Role)

	// 2. Create Event VIA HANDLER (to test logic in CreateEvent)
	app := fiber.New()
	repo := handlers.NewRepository(nil, db) // cfg is not used for this
	app.Post("/events", middleware.Protected, repo.CreateEvent)
	app.Post("/allocations", middleware.Protected, handlers.AllocateFamilyHandler(db))

	type RoomsInventoryItem struct {
		RoomOfferID  string `json:"room_offer_id"`
		RoomName     string `json:"room_name"`
		Total        int    `json:"total"`
		Available    int    `json:"available"`
		MaxCapacity  int    `json:"max_capacity"`
		PricePerRoom int    `json:"price_per_room"`
	}

	roomID := uuid.New().String()

	// Create Hotel first
	hotelID := "HOTEL1"
	hotel := models.Hotel{
		ID:     hotelID,
		CityID: "GOA", // Assuming this exists or isn't strict FK for test setup (setupTestDB creates Country/City usually? No, setupTestDB in allocation_api_test might not be called here fully if I didn't use it right).
		// Actually, let's look at setupTestDB in allocation_api_test.go which we are reusing.
		// It creates Country/City.
		Name:       "Test Hotel",
		StarRating: 5,
		Occupancy:  500,
	}
	// We assume Country/City exist from setupTestDB called at start of test?
	// setupTestDB clears DB but creates Country/City?
	// Let's check allocation_api_test.go content again.
	// Yes, seedTestData does it. But here I am NOT using seedTestData. I am manually creating data.
	// setupTestDB only creates tables.
	// So I need to create Country and City too if they are required.
	// Or simpler: I can just attempt to create Hotel and if it fails due to FK, I'll know.
	// Models usually have FKs.

	// Minimal dependency approach:
	country := models.Country{Code: "US", Name: "USA", PhoneCode: "+1"}
	db.Create(&country)
	city := models.City{ID: "NYC", CountryCode: "US", Name: "New York"}
	db.Create(&city)

	hotel.CityID = "NYC"
	db.Create(&hotel)

	// Create RoomOffer in DB because handler checks it
	roomOffer := models.RoomOffer{
		ID:          roomID,
		HotelID:     hotelID,
		Name:        "Suite",
		MaxCapacity: 4,
		TotalFare:   100,
		Currency:    "USD",
	}
	if err := db.Create(&roomOffer).Error; err != nil {
		t.Fatalf("Failed to create room offer: %v", err)
	}

	inventory := []RoomsInventoryItem{
		{RoomOfferID: roomID, RoomName: "Suite", Available: 10, MaxCapacity: 4, PricePerRoom: 100},
	}
	// Note: We do NOT send Total in request, to verify backend sets it.

	reqBody := map[string]interface{}{
		"name":           "Test Event",
		"hotelId":        "HOTEL1",
		"location":       "Test Loc",
		"startDate":      "2025-01-01",
		"endDate":        "2025-01-05",
		"roomsInventory": inventory,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/events", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// 3. Verify Event in DB has Total = 10
	var event models.Event
	db.First(&event) // Should be only one

	var dbInventory []RoomsInventoryItem
	json.Unmarshal(event.RoomsInventory, &dbInventory)

	assert.Equal(t, 1, len(dbInventory))
	assert.Equal(t, 10, dbInventory[0].Available)
	assert.Equal(t, 10, dbInventory[0].Total, "Total should be initialized to Available")

	// 4. Allocate a Family
	// Need a family and guests first
	familyID := uuid.New()
	guest := models.Guest{
		ID: uuid.New(), FamilyID: familyID, EventID: event.ID, Name: "G1", Age: 30, Type: "adult",
	}
	db.Create(&guest)

	allocReq := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     familyID.String(),
		"room_offer_id": roomID,
	}
	allocBody, _ := json.Marshal(allocReq)
	reqAlloc := httptest.NewRequest("POST", "/allocations", bytes.NewReader(allocBody))
	reqAlloc.Header.Set("Content-Type", "application/json")
	reqAlloc.Header.Set("Authorization", "Bearer "+token)

	respAlloc, err := app.Test(reqAlloc, -1)
	require.NoError(t, err)

	if respAlloc.StatusCode != fiber.StatusCreated {
		var body map[string]interface{}
		json.NewDecoder(respAlloc.Body).Decode(&body)
		t.Logf("Allocation Failed. Status: %d, Body: %+v", respAlloc.StatusCode, body)
	}
	require.Equal(t, fiber.StatusCreated, respAlloc.StatusCode)

	// 5. Verify Inventory Update
	db.First(&event, event.ID) // Reload
	json.Unmarshal(event.RoomsInventory, &dbInventory)

	t.Logf("DB Inventory Post-Allocation: %+v", dbInventory)

	assert.Equal(t, 9, dbInventory[0].Available, "Available should decrement")
	assert.Equal(t, 10, dbInventory[0].Total, "Total should REMAIN UNCHANGED")

	t.Log("✅ TestPassed: Inventory Total Persistence and Safety")
}
