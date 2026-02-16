package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config
var (
	BaseURL = "http://localhost:8080/api/v1"
	DB      *gorm.DB
	Token   string
	EventID uuid.UUID
)

func main() {
	fmt.Println("🔍 STARTING DIAGNOSTIC ANALYSIS SCRIPT")

	// 1. Connect to DB
	setupDB()

	// 2. Create User & Token
	setupAuth()

	// 3. Create Event & Guests
	setupData()

	// 4. Run Diagnostics
	fmt.Println("\n--- STEP 1 & 6: DATA SHAPE & INVENTORY ---")
	diagnoseDataShapes()

	fmt.Println("\n--- STEP 2: KEY PROP INVESTIGATION ---")
	diagnoseGuestIDs()

	fmt.Println("\n--- STEP 3: FAMILY GROUPING ---")
	diagnoseFamilyGrouping()

	fmt.Println("\n--- STEP 4: EVENT STATUS ---")
	diagnoseEventStatus()

	fmt.Println("\n--- STEP 5: INVALID EVENT ID ---")
	diagnoseMutationPayload()

	cleanup() // Optional, maybe keep for inspection?
}

func setupDB() {
	// Try loading env from various locations
	godotenv.Load("../../.env")
	godotenv.Load(".env")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		// Fallback for dev environment if not set
		dsn = "host=localhost user=postgres password=postgres dbname=tbo_dev port=5432 sslmode=disable"
		fmt.Println("⚠️ DATABASE_URL not found, trying default dev DSN:", dsn)
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ DB Connection Failed: %v", err)
	}
	fmt.Println("✅ DB Connected")
}

func setupAuth() {
	// Create Diagnostic Agent
	email := fmt.Sprintf("diag_%d@test.com", time.Now().Unix())
	pass := "diag123"
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)

	agent := models.User{
		ID:           uuid.New(),
		Name:         "Diagnostic Agent",
		Email:        email,
		PasswordHash: string(hash),
		Role:         "agent",
	}

	if err := DB.Create(&agent).Error; err != nil {
		log.Fatalf("❌ Agent Creation Failed: %v", err)
	}

	// Create Profile
	DB.Create(&models.AgentProfile{UserID: agent.ID, AgencyName: "Diag Agency"})

	// Generate Token
	var err error
	Token, err = utils.GenerateToken(agent.ID, agent.Email, agent.Role)
	if err != nil {
		log.Fatalf("❌ Token Gen Failed: %v", err)
	}
	fmt.Println("✅ Diagnostic Agent Created & Token Generated")
}

func setupData() {
	hotelID := "DIAG_HOTEL"
	// Ensure hotel exists
	DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Hotel{
		ID: hotelID, Name: "Diag Hotel", CityID: "NYC",
	})

	// Rooms
	rooms := []map[string]interface{}{
		{"room_offer_id": "RO_1", "room_name": "Standard", "max_capacity": 2, "total": 5, "available": 5},
	}
	invJSON, _ := json.Marshal(rooms)

	EventID = uuid.New()
	event := models.Event{
		ID:             EventID,
		Name:           "Diagnostic Event",
		HotelID:        hotelID,
		Status:         "draft", // Start as draft
		RoomsInventory: datatypes.JSON(invJSON),
		StartDate:      time.Now(),
		EndDate:        time.Now().Add(24 * time.Hour),
	}

	if err := DB.Create(&event).Error; err != nil {
		log.Fatalf("❌ Event Creation Failed: %v", err)
	}
	fmt.Println("✅ Diagnostic Event Created:", EventID)

	// Guests
	// Family 1: Valid
	f1 := uuid.New()
	createGuest("G1_Dad", f1)
	createGuest("G1_Mom", f1)

	// Family 2: "Unknown"? (Missing FamilyID?) - In Gorm it might be nil if pointer, or zero value
	// We'll create one with a new UUID but verify if API returns it
	f2 := uuid.New()
	createGuest("G2_Solo", f2)
}

func createGuest(name string, famID uuid.UUID) {
	guest := models.Guest{
		ID:       uuid.New(),
		EventID:  EventID,
		FamilyID: famID,
		Name:     name,
		Type:     "adult",
	}
	DB.Create(&guest)
}

func diagnoseDataShapes() {
	// Fetch Allocations (includes Inventory & Status)
	url := fmt.Sprintf("%s/events/%s/allocations", BaseURL, EventID)
	respBody := curl(url, "GET", nil)

	fmt.Println("👉 /allocations RESPONSE PAYLOAD:")
	fmt.Println(respBody)
}

func diagnoseGuestIDs() {
	url := fmt.Sprintf("%s/events/%s/guests", BaseURL, EventID)
	respBody := curl(url, "GET", nil)
	fmt.Println("👉 /guests RESPONSE PAYLOAD:")
	fmt.Println(respBody)
}

func diagnoseFamilyGrouping() {
	// Handled by inspecting /guests response above
	fmt.Println("ℹ️  Check /guests output above for 'family_id' casing (family_id vs familyId)")
}

func diagnoseEventStatus() {
	// Try to allocate in DRAFT status
	payload := map[string]string{
		"event_id":      EventID.String(),
		"family_id":     "some-uuid",
		"room_offer_id": "RO_1",
	}
	fmt.Println("👉 Attempting Allocation in DRAFT status:")
	res, _ := curlWithStatus(fmt.Sprintf("%s/allocations", BaseURL), "POST", payload)
	fmt.Println("Status Code:", res)
}

func diagnoseMutationPayload() {
	// We'll try two payloads
	// 1. snake_case keys (expected by Go typically?)
	// 2. camelCase keys (maybe JS sends this?)

	p1 := map[string]string{
		"event_id": EventID.String(), "family_id": "test", "room_offer_id": "test",
	}
	p2 := map[string]string{
		"eventId": EventID.String(), "familyId": "test", "roomOfferId": "test",
	}

	fmt.Println("👉 Payload Check (snake_case):")
	curlWithStatus(fmt.Sprintf("%s/allocations", BaseURL), "POST", p1)

	fmt.Println("👉 Payload Check (camelCase):")
	curlWithStatus(fmt.Sprintf("%s/allocations", BaseURL), "POST", p2)
}

func curl(url, method string, body interface{}) string {
	_, resp := curlWithStatus(url, method, body)
	return resp
}

func curlWithStatus(url, method string, body interface{}) (int, string) {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, _ := http.NewRequest(method, url, bodyReader)
	req.Header.Set("Authorization", "Bearer "+Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Request Failed: %v\n", err)
		return 0, ""
	}
	defer resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func cleanup() {
	// DB.Delete(&models.Event{}, EventID)
	// Keep for manual inspection if needed
}
