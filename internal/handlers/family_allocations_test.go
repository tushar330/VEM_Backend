package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
)

// Test database setup
func setupFamilyTestDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "Failed to connect to test database")

	// Auto-migrate
	err = db.AutoMigrate(
		&models.User{},
		&models.AgentProfile{},
		&models.Country{},
		&models.City{},
		&models.Hotel{},
		&models.RoomOffer{},
		&models.Event{},
		&models.Guest{},
		&models.GuestAllocation{},
	)
	require.NoError(t, err, "Failed to migrate test database")

	return db
}

// Seed test data
func seedFamilyTestData(t *testing.T, db *gorm.DB) (models.Event, []models.Guest, []models.RoomOffer) {
	// Clean up existing data
	db.Exec("TRUNCATE TABLE guest_allocations, guests, events, room_offers, hotels, cities, countries, agent_profiles, users RESTART IDENTITY CASCADE")

	// Create agent
	agent := models.User{
		ID:           uuid.New(),
		Name:         "Test Agent",
		Email:        "agent@test.com",
		PasswordHash: "hash",
		Role:         "agent",
	}
	db.Create(&agent)

	// Create head guest
	headGuest := models.User{
		ID:           uuid.New(),
		Name:         "Test Head Guest",
		Email:        "headguest@test.com",
		PasswordHash: "hash",
		Role:         "head_guest",
	}
	db.Create(&headGuest)

	// Create country and city
	country := models.Country{Code: "IN", Name: "India", PhoneCode: "+91"}
	db.Create(&country)

	city := models.City{ID: "TEST", CountryCode: "IN", Name: "Test City", IsPopular: true}
	db.Create(&city)

	// Create hotel
	facilities, _ := json.Marshal([]string{"WiFi", "Pool"})
	imageUrls, _ := json.Marshal([]string{"img1.jpg"})
	hotel := models.Hotel{
		ID:         "TEST001",
		CityID:     "TEST",
		Name:       "Test Hotel",
		StarRating: 5,
		Facilities: datatypes.JSON(facilities),
		ImageUrls:  datatypes.JSON(imageUrls),
		Occupancy:  100,
	}
	db.Create(&hotel)

	// Create room offers
	cancelPolicy, _ := json.Marshal([]map[string]interface{}{})
	roomOffers := []models.RoomOffer{
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Deluxe Room",
			BookingCode:    "DELUXE_001",
			MaxCapacity:    2,
			TotalFare:      5000.00,
			Currency:       "INR",
			IsRefundable:   true,
			CancelPolicies: datatypes.JSON(cancelPolicy),
		},
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Family Suite",
			BookingCode:    "FAMILY_001",
			MaxCapacity:    4,
			TotalFare:      8000.00,
			Currency:       "INR",
			IsRefundable:   true,
			CancelPolicies: datatypes.JSON(cancelPolicy),
		},
	}

	for _, room := range roomOffers {
		db.Create(&room)
	}

	// Create inventory
	inventory := []handlers.RoomsInventoryItem{
		{
			RoomOfferID:  roomOffers[0].ID,
			RoomName:     "Deluxe Room",
			Available:    2,
			MaxCapacity:  2,
			PricePerRoom: 5000,
		},
		{
			RoomOfferID:  roomOffers[1].ID,
			RoomName:     "Family Suite",
			Available:    1,
			MaxCapacity:  4,
			PricePerRoom: 8000,
		},
	}
	inventoryJSON, _ := json.Marshal(inventory)

	// Create event
	event := models.Event{
		ID:             uuid.New(),
		AgentID:        agent.ID,
		HeadGuestID:    headGuest.ID,
		HotelID:        hotel.ID,
		Name:           "Test Event",
		Location:       "Test Location",
		RoomsInventory: datatypes.JSON(inventoryJSON),
		Status:         "allocating",
		StartDate:      time.Now().Add(24 * time.Hour),
		EndDate:        time.Now().Add(48 * time.Hour),
	}
	db.Create(&event)

	// Create families
	family1ID := uuid.New()
	family2ID := uuid.New()

	guests := []models.Guest{
		// Family 1: 2 adults
		{
			ID:            uuid.New(),
			Name:          "Guest 1",
			Age:           30,
			Type:          "adult",
			EventID:       event.ID,
			FamilyID:      family1ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
		{
			ID:            uuid.New(),
			Name:          "Guest 2",
			Age:           28,
			Type:          "adult",
			EventID:       event.ID,
			FamilyID:      family1ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
		// Family 2: 4 people
		{
			ID:            uuid.New(),
			Name:          "Guest 3",
			Age:           35,
			Type:          "adult",
			EventID:       event.ID,
			FamilyID:      family2ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
		{
			ID:            uuid.New(),
			Name:          "Guest 4",
			Age:           33,
			Type:          "adult",
			EventID:       event.ID,
			FamilyID:      family2ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
		{
			ID:            uuid.New(),
			Name:          "Guest 5",
			Age:           10,
			Type:          "child",
			EventID:       event.ID,
			FamilyID:      family2ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
		{
			ID:            uuid.New(),
			Name:          "Guest 6",
			Age:           8,
			Type:          "child",
			EventID:       event.ID,
			FamilyID:      family2ID,
			ArrivalDate:   event.StartDate,
			DepartureDate: event.EndDate,
		},
	}

	for _, guest := range guests {
		db.Create(&guest)
	}

	return event, guests, roomOffers
}

// Test 1: Valid Allocation
func TestValidFamilyAllocation(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))

	// Allocate Family 1 (2 guests) to Deluxe Room (capacity 2)
	family1ID := guests[0].FamilyID
	reqBody := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family1ID.String(),
		"room_offer_id": roomOffers[0].ID,
		"assigned_by":   "agent_manual",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// Verify response
	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	assert.Equal(t, "Family allocated successfully", response["message"])
	assert.Equal(t, float64(2), response["family_size"])

	// Verify database
	var allocations []models.GuestAllocation
	db.Where("event_id = ? AND guest_id IN ?", event.ID, []uuid.UUID{guests[0].ID, guests[1].ID}).Find(&allocations)
	assert.Equal(t, 2, len(allocations))

	// Verify inventory decreased
	var updatedEvent models.Event
	db.First(&updatedEvent, event.ID)
	var inventory []handlers.RoomsInventoryItem
	json.Unmarshal(updatedEvent.RoomsInventory, &inventory)
	assert.Equal(t, 1, inventory[0].Available) // Was 2, now 1

	t.Log("✅ Test Passed: Valid Family Allocation")
}

// Test 2: Capacity Violation
func TestFamilyCapacityViolation(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))

	// Try to allocate Family 2 (4 guests) to Deluxe Room (capacity 2)
	family2ID := guests[2].FamilyID
	reqBody := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family2ID.String(),
		"room_offer_id": roomOffers[0].ID, // Deluxe Room with capacity 2
		"assigned_by":   "agent_manual",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	assert.Contains(t, response["error"], "exceeds room capacity")

	t.Log("✅ Test Passed: Capacity Violation Prevented")
}

// Test 3: Inventory Exhaustion
func TestFamilyInventoryExhaustion(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))

	// Allocate Family 1 to Deluxe Room (Available: 2)
	family1ID := guests[0].FamilyID
	reqBody1 := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family1ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes1, _ := json.Marshal(reqBody1)
	req1 := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := app.Test(req1)
	assert.Equal(t, fiber.StatusCreated, resp1.StatusCode)

	// Create another family
	family3ID := uuid.New()
	newGuests := []models.Guest{
		{Name: "Guest 7", Age: 40, Type: "adult", EventID: event.ID, FamilyID: family3ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
		{Name: "Guest 8", Age: 38, Type: "adult", EventID: event.ID, FamilyID: family3ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
	}
	for _, g := range newGuests {
		db.Create(&g)
	}

	// Allocate second family to same room type (Available: 1 after first allocation)
	reqBody2 := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family3ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)
	assert.Equal(t, fiber.StatusCreated, resp2.StatusCode)

	// Try to allocate third family (should fail - inventory exhausted)
	family4ID := uuid.New()
	newGuests2 := []models.Guest{
		{Name: "Guest 9", Age: 50, Type: "adult", EventID: event.ID, FamilyID: family4ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
		{Name: "Guest 10", Age: 48, Type: "adult", EventID: event.ID, FamilyID: family4ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
	}
	for _, g := range newGuests2 {
		db.Create(&g)
	}

	reqBody3 := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family4ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes3, _ := json.Marshal(reqBody3)
	req3 := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes3))
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := app.Test(req3)
	assert.Equal(t, fiber.StatusConflict, resp3.StatusCode)

	body, _ := io.ReadAll(resp3.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)
	assert.Contains(t, response["error"], "No rooms available")

	t.Log("✅ Test Passed: Inventory Exhaustion Handled")
}

// Test 4: Finalization Blocking
func TestFinalizationBlocking(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))
	app.Post("/events/:eventId/finalize", handlers.FinalizeRoomsHandler(db))

	// Finalize the event
	finalizeReq := httptest.NewRequest("POST", fmt.Sprintf("/events/%s/finalize", event.ID), nil)
	finalizeResp, _ := app.Test(finalizeReq)
	assert.Equal(t, fiber.StatusOK, finalizeResp.StatusCode)

	// Try to allocate after finalization (should fail)
	family1ID := guests[0].FamilyID
	reqBody := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family1ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)
	assert.Contains(t, response["error"], "rooms_finalized")

	t.Log("✅ Test Passed: Finalization Blocks Allocation")
}

// Test 5: Reopen Functionality
func TestReopenAllocation(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))
	app.Post("/events/:eventId/finalize", handlers.FinalizeRoomsHandler(db))
	app.Post("/events/:eventId/reopen", handlers.ReopenAllocationHandler(db))

	// Finalize
	finalizeReq := httptest.NewRequest("POST", fmt.Sprintf("/events/%s/finalize", event.ID), nil)
	app.Test(finalizeReq)

	// Reopen
	reopenReq := httptest.NewRequest("POST", fmt.Sprintf("/events/%s/reopen", event.ID), nil)
	reopenResp, _ := app.Test(reopenReq)
	assert.Equal(t, fiber.StatusOK, reopenResp.StatusCode)

	// Now allocation should work
	family1ID := guests[0].FamilyID
	reqBody := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family1ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	t.Log("✅ Test Passed: Reopen Allows Allocation")
}

// Test 6: Check-In Family
func TestCheckInFamily(t *testing.T) {
	db := setupFamilyTestDB(t)
	event, guests, roomOffers := seedFamilyTestData(t, db)

	app := fiber.New()
	app.Post("/allocate", handlers.AllocateFamilyHandler(db))
	app.Post("/events/:eventId/families/:familyId/checkin", handlers.CheckInFamilyHandler(db))

	// First allocate
	family1ID := guests[0].FamilyID
	reqBody := map[string]string{
		"event_id":      event.ID.String(),
		"family_id":     family1ID.String(),
		"room_offer_id": roomOffers[0].ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)
	allocReq := httptest.NewRequest("POST", "/allocate", bytes.NewReader(bodyBytes))
	allocReq.Header.Set("Content-Type", "application/json")
	app.Test(allocReq)

	// Check in
	checkinReq := httptest.NewRequest("POST", fmt.Sprintf("/events/%s/families/%s/checkin", event.ID, family1ID), nil)
	checkinResp, _ := app.Test(checkinReq)
	assert.Equal(t, fiber.StatusOK, checkinResp.StatusCode)

	// Verify status changed
	var allocations []models.GuestAllocation
	db.Where("event_id = ? AND guest_id IN ?", event.ID, []uuid.UUID{guests[0].ID, guests[1].ID}).Find(&allocations)
	for _, alloc := range allocations {
		assert.Equal(t, "checked_in", alloc.Status)
	}

	t.Log("✅ Test Passed: Family Check-In")
}
