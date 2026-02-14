package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/middleware"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Test data structure
type TestData struct {
	DB         *gorm.DB
	Agent      models.User
	HeadGuest  models.User
	Event      models.Event
	RoomOffers []models.RoomOffer
	Families   map[string]FamilyData
	AgentToken string
	HeadToken  string
}

type FamilyData struct {
	FamilyID uuid.UUID
	Guests   []models.Guest
}

// Setup test database
func setupTestDB(t *testing.T) *gorm.DB {
	// Load .env for DATABASE_URL
	godotenv.Load("../../.env")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL not set in environment")
	}

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
func seedTestData(t *testing.T, db *gorm.DB) *TestData {
	// Clean existing data
	db.Exec("TRUNCATE TABLE guest_allocations, guests, events, room_offers, hotels, cities, countries, agent_profiles, users RESTART IDENTITY CASCADE")

	// Create Agent
	agentPassword := "AgentPass123!"
	hashedAgentPass, _ := bcrypt.GenerateFromPassword([]byte(agentPassword), bcrypt.DefaultCost)
	agent := models.User{
		ID:           uuid.New(),
		Name:         "Rajesh Kumar",
		Email:        "rajesh.agent@tbo.com",
		PasswordHash: string(hashedAgentPass),
		Phone:        "+91-9876543210",
		Role:         "agent",
	}
	db.Create(&agent)

	// Create Head Guest
	headGuestPassword := "HeadGuestPass123!"
	hashedHeadPass, _ := bcrypt.GenerateFromPassword([]byte(headGuestPassword), bcrypt.DefaultCost)
	headGuest := models.User{
		ID:           uuid.New(),
		Name:         "Priya Sharma",
		Email:        "priya.headguest@example.com",
		PasswordHash: string(hashedHeadPass),
		Phone:        "+91-9123456789",
		Role:         "head_guest",
	}
	db.Create(&headGuest)

	// Create Country and City
	country := models.Country{Code: "IN", Name: "India", PhoneCode: "+91"}
	db.Create(&country)

	city := models.City{ID: "GOA", CountryCode: "IN", Name: "Goa", IsPopular: true}
	db.Create(&city)

	// Create Hotel
	facilities, _ := json.Marshal([]string{"WiFi", "Pool", "Spa"})
	imageUrls, _ := json.Marshal([]string{"https://example.com/hotel1.jpg"})
	hotel := models.Hotel{
		ID:         "GOA12345",
		CityID:     "GOA",
		Name:       "Paradise Beach Resort",
		StarRating: 5,
		Address:    "Calangute Beach Road, North Goa",
		Facilities: datatypes.JSON(facilities),
		ImageUrls:  datatypes.JSON(imageUrls),
		Occupancy:  500,
	}
	db.Create(&hotel)

	// Create Room Offers
	cancelPolicy, _ := json.Marshal([]map[string]interface{}{})
	roomOffers := []models.RoomOffer{
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Deluxe Ocean View",
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
			BookingCode:    "FAMILY_002",
			MaxCapacity:    4,
			TotalFare:      8000.00,
			Currency:       "INR",
			IsRefundable:   true,
			CancelPolicies: datatypes.JSON(cancelPolicy),
		},
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Premium Villa",
			BookingCode:    "VILLA_003",
			MaxCapacity:    6,
			TotalFare:      15000.00,
			Currency:       "INR",
			IsRefundable:   false,
			CancelPolicies: datatypes.JSON(cancelPolicy),
		},
	}

	for i := range roomOffers {
		db.Create(&roomOffers[i])
	}

	// Create Inventory
	type RoomsInventoryItem struct {
		RoomOfferID  string `json:"room_offer_id"`
		RoomName     string `json:"room_name"`
		Available    int    `json:"available"`
		MaxCapacity  int    `json:"max_capacity"`
		PricePerRoom int    `json:"price_per_room"`
	}

	inventory := []RoomsInventoryItem{
		{RoomOfferID: roomOffers[0].ID, RoomName: "Deluxe Ocean View", Available: 5, MaxCapacity: 2, PricePerRoom: 5000},
		{RoomOfferID: roomOffers[1].ID, RoomName: "Family Suite", Available: 3, MaxCapacity: 4, PricePerRoom: 8000},
		{RoomOfferID: roomOffers[2].ID, RoomName: "Premium Villa", Available: 2, MaxCapacity: 6, PricePerRoom: 15000},
	}
	inventoryJSON, _ := json.Marshal(inventory)

	// Create Event
	event := models.Event{
		ID:             uuid.New(),
		AgentID:        agent.ID,
		HeadGuestID:    headGuest.ID,
		HotelID:        hotel.ID,
		Name:           "Sharma Family Wedding",
		Location:       "Goa, India",
		RoomsInventory: datatypes.JSON(inventoryJSON),
		Status:         "allocating",
		StartDate:      time.Now().Add(30 * 24 * time.Hour),
		EndDate:        time.Now().Add(33 * 24 * time.Hour),
	}
	db.Create(&event)

	// Create Families
	families := make(map[string]FamilyData)

	// Family A: 2 guests (fits Deluxe)
	familyA := FamilyData{
		FamilyID: uuid.New(),
		Guests: []models.Guest{
			{Name: "Amit Verma", Age: 35, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Sneha Verma", Age: 32, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
		},
	}

	// Family B: 4 guests (fits Family Suite)
	familyB := FamilyData{
		FamilyID: uuid.New(),
		Guests: []models.Guest{
			{Name: "Vikram Singh", Age: 40, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Meera Singh", Age: 38, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Rohan Singh", Age: 10, Type: "child", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Aarav Singh", Age: 7, Type: "child", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
		},
	}

	// Family C: 3 guests (fits Family Suite)
	familyC := FamilyData{
		FamilyID: uuid.New(),
		Guests: []models.Guest{
			{Name: "Karan Patel", Age: 45, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Anjali Patel", Age: 42, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
			{Name: "Ravi Patel", Age: 65, Type: "adult", EventID: event.ID, ArrivalDate: event.StartDate, DepartureDate: event.EndDate},
		},
	}

	// Insert guests
	for _, family := range []FamilyData{familyA, familyB, familyC} {
		for i := range family.Guests {
			family.Guests[i].FamilyID = family.FamilyID
			db.Create(&family.Guests[i])
		}
	}

	families["A"] = familyA
	families["B"] = familyB
	families["C"] = familyC

	// Generate JWT tokens
	agentToken, _ := utils.GenerateToken(agent.ID, agent.Email, agent.Role)
	headToken, _ := utils.GenerateToken(headGuest.ID, headGuest.Email, headGuest.Role)

	return &TestData{
		DB:         db,
		Agent:      agent,
		HeadGuest:  headGuest,
		Event:      event,
		RoomOffers: roomOffers,
		Families:   families,
		AgentToken: agentToken,
		HeadToken:  headToken,
	}
}

// Create Fiber app with routes
func createTestApp(db *gorm.DB) *fiber.App {
	app := fiber.New()
	app.Post("/allocations", middleware.Protected, handlers.AllocateFamilyHandler(db))
	return app
}

// ============================================================================
// TESTS
// ============================================================================

func TestAgentValidAllocation(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	familyA := data.Families["A"]
	deluxeRoom := data.RoomOffers[0]

	reqBody := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyA.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+data.AgentToken)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode, "Should return 201 Created")

	// Verify assigned_mode
	var allocations []models.GuestAllocation
	db.Where("event_id = ?", data.Event.ID).Find(&allocations)
	assert.Equal(t, 2, len(allocations), "Should create 2 allocations")
	assert.Equal(t, "agent_manual", allocations[0].AssignedMode, "Should be agent_manual")

	// Verify inventory decreased
	var event models.Event
	db.First(&event, data.Event.ID)
	var inventory []map[string]interface{}
	json.Unmarshal(event.RoomsInventory, &inventory)
	assert.Equal(t, float64(4), inventory[0]["available"], "Inventory should decrease from 5 to 4")

	t.Log("✅ Test Passed: Agent Valid Allocation")
}

func TestHeadGuestValidAllocation(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	familyC := data.Families["C"]
	familySuite := data.RoomOffers[1]

	reqBody := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyC.FamilyID.String(),
		"room_offer_id": familySuite.ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+data.HeadToken)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// Verify assigned_mode
	var allocations []models.GuestAllocation
	db.Where("event_id = ?", data.Event.ID).Find(&allocations)
	assert.Equal(t, "head_guest_manual", allocations[0].AssignedMode, "Should be head_guest_manual")

	t.Log("✅ Test Passed: Head Guest Valid Allocation")
}

func TestUnauthorizedAgent(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	// Create second agent
	secondAgent := models.User{
		ID:    uuid.New(),
		Name:  "Another Agent",
		Email: "other@agent.com",
		Role:  "agent",
	}
	db.Create(&secondAgent)
	otherToken, _ := utils.GenerateToken(secondAgent.ID, secondAgent.Email, secondAgent.Role)

	familyA := data.Families["A"]
	deluxeRoom := data.RoomOffers[0]

	reqBody := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyA.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode, "Should return 403 Forbidden")

	t.Log("✅ Test Passed: Unauthorized Agent Blocked")
}

func TestCapacityViolation(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	familyB := data.Families["B"]    // 4 guests
	deluxeRoom := data.RoomOffers[0] // capacity 2

	reqBody := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyB.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+data.AgentToken)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode, "Should return 400 Bad Request")

	t.Log("✅ Test Passed: Capacity Violation Prevented")
}

func TestInventoryExhaustion(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	// Manually set inventory to 1
	type RoomsInventoryItem struct {
		RoomOfferID  string `json:"room_offer_id"`
		RoomName     string `json:"room_name"`
		Available    int    `json:"available"`
		MaxCapacity  int    `json:"max_capacity"`
		PricePerRoom int    `json:"price_per_room"`
	}

	inventory := []RoomsInventoryItem{
		{RoomOfferID: data.RoomOffers[0].ID, RoomName: "Deluxe Ocean View", Available: 1, MaxCapacity: 2, PricePerRoom: 5000},
		{RoomOfferID: data.RoomOffers[1].ID, RoomName: "Family Suite", Available: 3, MaxCapacity: 4, PricePerRoom: 8000},
		{RoomOfferID: data.RoomOffers[2].ID, RoomName: "Premium Villa", Available: 2, MaxCapacity: 6, PricePerRoom: 15000},
	}
	inventoryJSON, _ := json.Marshal(inventory)
	db.Model(&data.Event).Update("rooms_inventory", datatypes.JSON(inventoryJSON))

	familyA := data.Families["A"]
	deluxeRoom := data.RoomOffers[0]

	// First allocation (should succeed)
	reqBody1 := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyA.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes1, _ := json.Marshal(reqBody1)
	req1 := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+data.AgentToken)
	resp1, _ := app.Test(req1, -1)
	assert.Equal(t, fiber.StatusCreated, resp1.StatusCode)

	// Second allocation (should fail - inventory exhausted)
	// Create new family for second attempt
	familyD := FamilyData{
		FamilyID: uuid.New(),
		Guests: []models.Guest{
			{Name: "Test User 1", Age: 30, Type: "adult", EventID: data.Event.ID, FamilyID: uuid.New(), ArrivalDate: data.Event.StartDate, DepartureDate: data.Event.EndDate},
			{Name: "Test User 2", Age: 28, Type: "adult", EventID: data.Event.ID, FamilyID: uuid.New(), ArrivalDate: data.Event.StartDate, DepartureDate: data.Event.EndDate},
		},
	}
	for i := range familyD.Guests {
		familyD.Guests[i].FamilyID = familyD.FamilyID
		db.Create(&familyD.Guests[i])
	}

	reqBody2 := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyD.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+data.AgentToken)
	resp2, _ := app.Test(req2, -1)

	assert.Equal(t, fiber.StatusConflict, resp2.StatusCode, "Should return 409 Conflict")

	t.Log("✅ Test Passed: Inventory Exhaustion Handled")
}

func TestMissingAuthToken(t *testing.T) {
	db := setupTestDB(t)
	data := seedTestData(t, db)
	app := createTestApp(db)

	familyA := data.Families["A"]
	deluxeRoom := data.RoomOffers[0]

	reqBody := map[string]string{
		"event_id":      data.Event.ID.String(),
		"family_id":     familyA.FamilyID.String(),
		"room_offer_id": deluxeRoom.ID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/allocations", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// NO Authorization header

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode, "Should return 401 Unauthorized")

	t.Log("✅ Test Passed: Missing Auth Token Rejected")
}

func TestAllocationAPIComplete(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🧪 ALLOCATION API TEST SUITE")
	fmt.Println(strings.Repeat("=", 60))

	t.Run("Agent Valid Allocation", TestAgentValidAllocation)
	t.Run("Head Guest Valid Allocation", TestHeadGuestValidAllocation)
	t.Run("Unauthorized Agent", TestUnauthorizedAgent)
	t.Run("Capacity Violation", TestCapacityViolation)
	t.Run("Inventory Exhaustion", TestInventoryExhaustion)
	t.Run("Missing Auth Token", TestMissingAuthToken)

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("✅ ALL ALLOCATION API TESTS COMPLETED")
	fmt.Println(strings.Repeat("=", 60) + "\n")
}
