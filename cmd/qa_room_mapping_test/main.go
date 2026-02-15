package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/config"
	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/routes"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// ==========================================
// TEST CONFIGURATION & STATE
// ==========================================
var (
	DB         *gorm.DB
	App        *fiber.App
	AgentToken string
	Agent      models.User
	Event      models.Event
	Guests     []models.Guest
	Families   map[string][]models.Guest
	Inventory  []handlers.RoomsInventoryItem
	RoomOffers []models.RoomOffer

	// Test Results
	TotalTests    int
	PassedTests   int
	FailedTests   int
	FailedReasons []string
)

// logPass prints a success message
func logPass(testName string) {
	fmt.Printf("✅ [PASS] %s\n", testName)
	TotalTests++
	PassedTests++
}

// logFail prints a failure message and tracks it
func logFail(testName, reason string) {
	fmt.Printf("❌ [FAIL] %s: %s\n", testName, reason)
	TotalTests++
	FailedTests++
	FailedReasons = append(FailedReasons, fmt.Sprintf("%s: %s", testName, reason))
}

// fatalFail prints a failure message and exits
func fatalFail(step, reason string) {
	fmt.Printf("\n🚨 CRITICAL FAILURE in [%s]: %s\n", step, reason)
	cleanup()
	os.Exit(1)
}

// ==========================================
// SETUP & SEEDING
// ==========================================
func setup() {
	// Load .env
	if err := godotenv.Load("../../.env"); err != nil {
		if err := godotenv.Load(".env"); err != nil {
			log.Println("⚠️  No .env file found, checking env vars")
		}
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fatalFail("Setup", "DATABASE_URL is not set")
	}

	var err error
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			LogLevel: logger.Silent,
		},
	)
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		fatalFail("Setup", "Failed to connect to database: "+err.Error())
	}

	// Initialize Fiber App
	cfg := &config.Config{Env: "test"}
	repo := handlers.NewRepository(cfg, DB)
	App = fiber.New()
	routes.SetupRoutes(App, cfg, repo)

	fmt.Println("🚀 Setup Complete: DB Connected & App Initialized")
}

func seedData() {
	fmt.Println("🌱 Seeding Test Data...")

	timestamp := time.Now().Unix()

	// 1. Create Agent
	agentEmail := fmt.Sprintf("qa_agent_%d@test.com", timestamp)
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	Agent = models.User{
		ID:           uuid.New(),
		Name:         fmt.Sprintf("QA Test Agent %d", timestamp),
		Email:        agentEmail,
		PasswordHash: string(passwordHash),
		Role:         "agent",
		Phone:        "+919876543210",
	}
	if err := DB.Create(&Agent).Error; err != nil {
		fatalFail("Seeding", "Failed to create agent: "+err.Error())
	}
	DB.Create(&models.AgentProfile{UserID: Agent.ID, AgencyName: "QA Agency"})

	// Generate Token
	token, err := utils.GenerateToken(Agent.ID, Agent.Email, Agent.Role)
	if err != nil {
		fatalFail("Seeding", "Failed to generate token: "+err.Error())
	}
	AgentToken = token

	// 2. Create Hotel & Rooms
	hotelID := fmt.Sprintf("QA_HOTEL_%d", timestamp)
	country := models.Country{Code: "QA", Name: "QA Land", PhoneCode: "00"}
	DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&country)
	city := models.City{ID: "QA_CITY", CountryCode: "QA", Name: "QA City"}
	DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&city)

	hotel := models.Hotel{
		ID:         hotelID,
		CityID:     "QA_CITY",
		Name:       "QA Test Hotel",
		StarRating: 5,
	}
	DB.Create(&hotel)

	// Create 3 Room Offers
	RoomOffers = []models.RoomOffer{
		{ID: uuid.New().String(), HotelID: hotelID, Name: "Standard Room", MaxCapacity: 2, TotalFare: 100, Count: 20},
		{ID: uuid.New().String(), HotelID: hotelID, Name: "Family Room", MaxCapacity: 4, TotalFare: 200, Count: 10},
		{ID: uuid.New().String(), HotelID: hotelID, Name: "large Suite", MaxCapacity: 6, TotalFare: 500, Count: 5},
	}
	DB.Create(&RoomOffers)

	// 3. Create Inventory JSON
	Inventory = []handlers.RoomsInventoryItem{
		{RoomOfferID: RoomOffers[0].ID, RoomName: RoomOffers[0].Name, Available: 20, MaxCapacity: 2, PricePerRoom: 100},
		{RoomOfferID: RoomOffers[1].ID, RoomName: RoomOffers[1].Name, Available: 10, MaxCapacity: 4, PricePerRoom: 200},
		{RoomOfferID: RoomOffers[2].ID, RoomName: RoomOffers[2].Name, Available: 5, MaxCapacity: 6, PricePerRoom: 500},
	}
	invJSON, _ := json.Marshal(Inventory)

	// 4. Create Event
	Event = models.Event{
		ID:             uuid.New(),
		AgentID:        Agent.ID,
		Name:           "QA Room Mapping Test Event",
		HotelID:        hotelID,
		Status:         "allocating",
		RoomsInventory: datatypes.JSON(invJSON),
		StartDate:      time.Now(),
		EndDate:        time.Now().Add(24 * time.Hour),
	}
	if err := DB.Create(&Event).Error; err != nil {
		fatalFail("Seeding", "Failed to create event: "+err.Error())
	}

	// 5. Create Guests (Families & Singles)
	createFamily(2, "Standard Fam")
	createFamily(4, "Family Fam")
	createFamily(3, "Triple Fam")
	createFamily(1, "Single Guy")

	fmt.Printf("✅ Seeded: Agent %s, Event %s, %d Guests\n", Agent.ID, Event.ID, len(Guests))

	var dbGuestCount int64
	DB.Model(&models.Guest{}).Where("event_id = ?", Event.ID).Count(&dbGuestCount)
	fmt.Printf("🔍 DB Verification: Found %d guests in DB for Event %s\n", dbGuestCount, Event.ID)
}

func createFamily(size int, prefix string) {
	famID := uuid.New()
	for i := 0; i < size; i++ {
		guest := models.Guest{
			ID:            uuid.New(),
			EventID:       Event.ID,
			FamilyID:      famID,
			Name:          fmt.Sprintf("%s Member %d", prefix, i+1),
			Age:           30,
			Type:          "adult",
			ArrivalDate:   Event.StartDate,
			DepartureDate: Event.EndDate,
		}
		Guests = append(Guests, guest)
	}
	start := len(Guests) - size
	if start < 0 {
		start = 0
	}
	newGuests := Guests[start:]
	DB.Create(&newGuests)
}

func cleanup() {
	if os.Getenv("CLEANUP") != "true" {
		fmt.Println("\n🧹 Cleanup skipped (Set CLEANUP=true to enable)")
		printManualVerificationData()
		return
	}
	fmt.Println("\n🧹 Cleaning up test data...")
	DB.Where("event_id = ?", Event.ID).Delete(&models.GuestAllocation{})
	DB.Where("event_id = ?", Event.ID).Delete(&models.Guest{})
	DB.Delete(&Event)
	DB.Delete(&Agent)
	fmt.Println("✅ Cleanup Complete")
}

func printManualVerificationData() {
	// Disabled for brevity
}

// ==========================================
// API HELPERS
// ==========================================
func makeRequest(method, url string, body interface{}) (*http.Response, map[string]interface{}) {
	var bodyReader *bytes.Reader
	if body != nil {
		reqBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(reqBytes)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req := httptest.NewRequest(method, url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+AgentToken)

	// fmt.Printf("DEBUG: calling %s %s\n", method, url)
	fromApp, err := App.Test(req, 10000)
	// fmt.Printf("DEBUG: call finished\n")
	if err != nil {
		fatalFail("API Request", fmt.Sprintf("%s %s call failed: %v", method, url, err))
	}

	var respBody map[string]interface{}
	json.NewDecoder(fromApp.Body).Decode(&respBody)

	return fromApp, respBody
}

func getInitialInventory(offerID string) int {
	for _, item := range Inventory {
		if item.RoomOfferID == offerID {
			return item.Available
		}
	}
	return 0
}

// ==========================================
// TEST IMPLEMENTATIONS
// ==========================================

func runAutoAllocationTest() {
	fmt.Println("\n🧪 Testing Auto Allocation...")

	resp, body := makeRequest("POST", "/api/v1/events/"+Event.ID.String()+"/auto-allocate", nil)

	if resp.StatusCode != 200 {
		logFail("AutoAllocate", fmt.Sprintf("Status %d - %v", resp.StatusCode, body))
		return
	}

	var msg string
	var ok bool

	if m, o := body["message"].(string); o {
		msg = m
		ok = true
	} else if data, o := body["data"].(map[string]interface{}); o {
		if m, o := data["message"].(string); o {
			msg = m
			ok = true
		}
	}

	if ok && strings.Contains(msg, "completed") {
		logPass("AutoAllocate API Success")
	} else {
		logFail("AutoAllocate", "Unexpected message: "+msg)
	}

	var count int64
	DB.Model(&models.GuestAllocation{}).Where("event_id = ?", Event.ID).Count(&count)
	if int(count) == len(Guests) {
		logPass("All guests allocated in DB")
	} else {
		logFail("AutoAllocate Verification", fmt.Sprintf("Expected %d allocations, found %d", len(Guests), count))
	}

	var updatedEvent models.Event
	DB.First(&updatedEvent, Event.ID)
	var inv []handlers.RoomsInventoryItem
	json.Unmarshal(updatedEvent.RoomsInventory, &inv)

	decremented := false
	for _, item := range inv {
		if item.Available < getInitialInventory(item.RoomOfferID) {
			decremented = true
		}
	}
	if decremented {
		logPass("Inventory Decremented")
	} else {
		logFail("Inventory Check", "Inventory did not decrease")
	}
}

func runPageLoadAPIsTest() {
	fmt.Println("\n🧪 Testing Page Load APIs...")

	// 1. GET /events/:id
	resp, body := makeRequest("GET", "/api/v1/events/"+Event.ID.String(), nil)
	if resp.StatusCode != 200 {
		logFail("GET /events/:id", fmt.Sprintf("Status %d", resp.StatusCode))
	} else {
		var eventFound bool
		// Check data.event.rooms_inventory
		if data, ok := body["data"].(map[string]interface{}); ok {
			if eventMap, ok := data["event"].(map[string]interface{}); ok {
				eventFound = true
				if _, ok := eventMap["rooms_inventory"]; ok {
					logPass("GET /events/:id returns inventory")
				} else {
					logFail("GET /events/:id", "Missing rooms_inventory")
				}
			}
		}
		if !eventFound {
			logFail("GET /events/:id", "Missing event object or invalid structure")
		}
	}

	// 2. GET /events/:id/guests
	resp, body = makeRequest("GET", "/api/v1/events/"+Event.ID.String()+"/guests", nil)
	if resp.StatusCode != 200 {
		logFail("GETGuests", fmt.Sprintf("Status %d", resp.StatusCode))
	} else {
		var guestsFound bool
		if data, ok := body["data"].(map[string]interface{}); ok {
			if guestsList, ok := data["guests"].([]interface{}); ok {
				guestsFound = true
				if len(guestsList) == len(Guests) {
					logPass("GETGuests returns all guests")
				} else {
					logFail("GETGuests", fmt.Sprintf("Expected %d guests, got %d", len(Guests), len(guestsList)))
				}
			}
		}
		if !guestsFound {
			logFail("GETGuests", "guests field is not a list or missing")
		}
	}

	// 3. GET /events/:id/allocations
	resp, body = makeRequest("GET", "/api/v1/events/"+Event.ID.String()+"/allocations", nil)
	if resp.StatusCode != 200 {
		logFail("GETAllocations", fmt.Sprintf("Status %d", resp.StatusCode))
	} else {
		var allocs []interface{}
		var found bool

		if data, ok := body["data"].(map[string]interface{}); ok {
			if list, ok := data["allocations"].([]interface{}); ok {
				allocs = list
				found = true
			}
		}

		if !found {
			if list, ok := body["allocations"].([]interface{}); ok {
				allocs = list
				found = true
			}
		}

		if found {
			if len(allocs) == 0 {
				logPass("GETAllocations initially empty")
			} else {
				logFail("GETAllocations", "Should be empty initially")
			}
		} else {
			logFail("GETAllocations", "Missing or invalid allocations field")
		}
	}
}

func runManualAssignmentTest() {
	fmt.Println("\n🧪 Testing Manual Assignment...")

	// Create a new guest who is NOT allocated
	newGuestID := uuid.New()
	newFamilyID := uuid.New()
	newGuest := models.Guest{
		ID:            newGuestID,
		EventID:       Event.ID,
		FamilyID:      newFamilyID,
		Name:          "Manual Guest",
		Age:           25,
		Type:          "adult",
		ArrivalDate:   Event.StartDate,
		DepartureDate: Event.EndDate,
	}
	if err := DB.Create(&newGuest).Error; err != nil {
		logFail("Manual Allocation Setup", "Failed to create guest: "+err.Error())
		return
	}
	// Do NOT append to Guests global slice because AutoAllocate checks DB/Guests consistency based on INITIAL seed.
	// But verification below checks DB update for this guest.

	targetRoom := RoomOffers[2] // Large Suite

	payload := map[string]string{
		"event_id":      Event.ID.String(),
		"family_id":     newGuest.FamilyID.String(),
		"room_offer_id": targetRoom.ID,
	}

	resp, body := makeRequest("POST", "/api/v1/allocations", payload)

	if resp.StatusCode == 201 {
		logPass("Manual Allocation API Success")
	} else {
		logFail("Manual Allocation", fmt.Sprintf("Status %d - %v", resp.StatusCode, body))
	}

	var alloc models.GuestAllocation
	DB.Where("guest_id = ?", newGuest.ID).First(&alloc)
	if alloc.RoomOfferID != nil && *alloc.RoomOfferID == targetRoom.ID {
		logPass("DB Updated Correctly")
	} else {
		logFail("Manual Verification", "Guest room ID not updated")
	}

	if alloc.AssignedMode == "agent_manual" {
		logPass("AssignedMode updated to agent_manual")
	} else {
		logFail("AssignedMode", "Expected agent_manual, got "+alloc.AssignedMode)
	}
}

func runReallocationTest() {
	fmt.Println("\n🧪 Testing Reallocation...")

	// Pick an Auto-Allocated Guest (Single Guy)
	var singleGuest models.Guest
	DB.Where("event_id = ? AND name = ?", Event.ID, "Single Guy Member 1").First(&singleGuest)

	var alloc models.GuestAllocation
	DB.Where("guest_id = ?", singleGuest.ID).First(&alloc)

	// Move to Family Room (Different from Standard)
	targetRoom := RoomOffers[1]

	payload := map[string]string{
		"room_offer_id": targetRoom.ID,
	}

	resp, body := makeRequest("PUT", "/api/v1/allocations/"+alloc.ID.String(), payload)

	if resp.StatusCode == 200 {
		logPass("Reallocation API Success")
	} else {
		logFail("Reallocation", fmt.Sprintf("Status %d - %v", resp.StatusCode, body))
	}

	DB.First(&alloc, alloc.ID)
	if alloc.RoomOfferID != nil && *alloc.RoomOfferID == targetRoom.ID {
		logPass("Reallocation DB Verify")
	} else {
		logFail("Reallocation Verify", fmt.Sprintf("Expected %s, got %v", targetRoom.ID, alloc.RoomOfferID))
	}

	if alloc.AssignedMode == "agent_manual_update" {
		logPass("AssignedMode updated to agent_manual_update")
	} else {
		logFail("AssignedMode", "Expected agent_manual_update, got "+alloc.AssignedMode)
	}
}

func main() {
	fmt.Println("🚀 STARTING ROOM MAPPING QA SCRIPT 🚀")
	setup()
	seedData()

	runPageLoadAPIsTest()
	runAutoAllocationTest()
	runManualAssignmentTest()
	runReallocationTest()

	fmt.Println("\n============================")
	fmt.Printf("ROOM MAPPING QA SUMMARY\n")
	fmt.Printf("TOTAL TESTS: %d\n", TotalTests)
	fmt.Printf("PASSED:      %d\n", PassedTests)
	fmt.Printf("FAILED:      %d\n", FailedTests)
	if len(FailedReasons) > 0 {
		fmt.Println("FAILURES:")
		for _, r := range FailedReasons {
			fmt.Printf(" - %s\n", r)
		}
	}
	fmt.Println("============================")

	cleanup()

	if FailedTests > 0 {
		os.Exit(1)
	}
}
