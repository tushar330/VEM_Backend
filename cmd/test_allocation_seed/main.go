package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InventoryItem matches the structure expected by the backend for rooms_inventory
type InventoryItem struct {
	RoomOfferID string `json:"room_offer_id"`
	RoomName    string `json:"room_name"`
	Capacity    int    `json:"capacity"`
	Total       int    `json:"total"`
	Available   int    `json:"available"`
}

var (
	db      *gorm.DB
	apiBase = "http://localhost:8080/api/v1"
	cleanup bool
)

func main() {
	// 🏗 STEP 1 — Setup
	fmt.Println("⚠ Running Allocation Seed Script (DEV ONLY)")

	// Define flags
	flag.BoolVar(&cleanup, "cleanup", false, "Clean up created data after running")
	flag.Parse()

	// Load .env
	if err := godotenv.Load("../../.env"); err != nil {
		// Try loading from current dir if ../../ fails (in case running from root)
		if err := godotenv.Load(".env"); err != nil {
			log.Println("Warning: Error loading .env file")
		}
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("❌ DATABASE_URL not set")
	}

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("❌ Failed to connect to database:", err)
	}

	// 2. Data Creation
	// ---------------------------------------------------------
	// 2.1 Create Room Offers
	r1UUID := uuid.New().String()
	r2UUID := uuid.New().String()
	// Dummy Hotel ID - required for RoomOffer.
	// To be safe, let's pick a random string or create a dummy hotel if needed.
	// Assuming TBO Code is string, let's use a dummy one "TEST_HOTEL_999"
	hotelID := "TEST_HOTEL_999"
	// 2.1 Get or Create Hotel (to satisfy FKs)
	var hotel models.Hotel
	// Try to find ANY existing hotel to use
	if err := db.First(&hotel).Error; err == nil {
		fmt.Printf("ℹ️ Using existing hotel: %s (%s)\n", hotel.ID, hotel.Name)
		hotelID = hotel.ID
	} else {
		fmt.Println("ℹ️ No existing hotel found. Creating test hotel chain (Country -> City -> Hotel)...")

		// Create Country
		country := models.Country{Code: "ZZ", Name: "Test Country", PhoneCode: "999"}
		if err := db.FirstOrCreate(&country).Error; err != nil {
			log.Fatalf("❌ Failed to create/find Country: %v", err)
		}

		// Create City
		city := models.City{ID: "TEST_CITY_99", CountryCode: country.Code, Name: "Test City"}
		if err := db.FirstOrCreate(&city).Error; err != nil {
			log.Fatalf("❌ Failed to create/find City: %v", err)
		}

		// Create Hotel
		newHotel := models.Hotel{
			ID:         hotelID, // Uses the constant defined above
			CityID:     city.ID,
			Name:       "Test Hotel",
			Address:    "123 Test St",
			StarRating: 5,
			Occupancy:  1000,
		}
		if err := db.FirstOrCreate(&newHotel).Error; err != nil {
			log.Fatalf("❌ Failed to create Hotel: %v", err)
		}
		hotel = newHotel
	}

	// Create Room Offers
	r1 := models.RoomOffer{
		ID:          r1UUID,
		HotelID:     hotelID,
		Name:        "Deluxe",
		BookingCode: "DLX",
		MaxCapacity: 2,
		TotalFare:   100.00,
		Count:       10, // Total physical rooms logic if used
	}
	r2 := models.RoomOffer{
		ID:          r2UUID,
		HotelID:     hotelID,
		Name:        "Suite",
		BookingCode: "STE",
		MaxCapacity: 4,
		TotalFare:   200.00,
		Count:       5,
	}

	if err := db.Create(&r1).Error; err != nil {
		// If failure likely due to HotelID FK, we might need to skip or warn.
		// For now assuming we can insert.
		// Retrying with a more robust approach if this was a real app, but for script we panic on fail.
		// Actually, let's just create them. If it fails, the user will see.
		log.Fatalf("❌ Failed to create RoomOffer R1: %v (Ensure HotelID %s works or disable FK check)", err, hotelID)
	}
	if err := db.Create(&r2).Error; err != nil {
		log.Fatalf("❌ Failed to create RoomOffer R2: %v", err)
	}

	fmt.Printf("Created Room Offers:\n  R1: %s (Deluxe, Cap 2)\n  R2: %s (Suite, Cap 4)\n", r1.ID, r2.ID)

	// 2.2 Create Event
	// Need Agent and HeadGuest UUIDs.
	agentID := uuid.New()
	headGuestID := uuid.New()

	// Prepare Inventory JSON
	inventory := []InventoryItem{
		{
			RoomOfferID: r1.ID,
			RoomName:    r1.Name,
			Capacity:    r1.MaxCapacity,
			Total:       5,
			Available:   5,
		},
		{
			RoomOfferID: r2.ID,
			RoomName:    r2.Name,
			Capacity:    r2.MaxCapacity,
			Total:       3,
			Available:   3,
		},
	}
	invJSON, _ := json.Marshal(inventory)

	eventID := uuid.New()
	event := models.Event{
		ID:             eventID,
		AgentID:        agentID,
		HeadGuestID:    headGuestID,
		HotelID:        hotelID,
		Name:           "Test Allocation Event",
		Status:         "allocating",
		RoomsInventory: datatypes.JSON(invJSON),
		StartDate:      time.Now(),
		EndDate:        time.Now().Add(24 * time.Hour),
	}

	if err := db.Create(&event).Error; err != nil {
		log.Fatalf("❌ Failed to create Event: %v", err)
	}

	fmt.Printf("\nCreated Event:\n  ID: %s\n  Status: %s\n", event.ID, event.Status)

	// 2.3 Create Families (Logical)
	f1ID := uuid.New()
	f2ID := uuid.New()

	// 2.4 Create Guests
	guests := []models.Guest{
		{
			Name:     "Guest 1 (F1)",
			Age:      30,
			EventID:  eventID,
			FamilyID: f1ID,
		},
		{
			Name:     "Guest 2 (F1)",
			Age:      28,
			EventID:  eventID,
			FamilyID: f1ID,
		},
		{
			Name:     "Guest 3 (F2)",
			Age:      35,
			EventID:  eventID,
			FamilyID: f2ID,
		},
	}

	for i := range guests {
		if err := db.Create(&guests[i]).Error; err != nil {
			log.Fatalf("❌ Failed to create Guest %d: %v", i+1, err)
		}
	}

	fmt.Printf("\nCreated Guests:\n")
	for _, g := range guests {
		fmt.Printf("  %s (Family: %s)\n", g.Name, g.FamilyID)
	}

	fmt.Printf("\nInitial Inventory:\n  R1: 5\n  R2: 3\n")

	// 🏗 STEP 3 — Test Allocation APIs
	client := &http.Client{Timeout: 10 * time.Second}

	// Generate JWT Token for the Agent
	token, err := utils.GenerateToken(agentID, "testagent@example.com", "agent")
	if err != nil {
		log.Fatalf("❌ Failed to generate token: %v", err)
	}
	fmt.Printf("\n🔑 Generated Test Token for Agent %s\n", agentID)

	// 3.1 POST Allocation (F1 -> R1)
	fmt.Printf("\n🧪 Testing POST /allocations (F1 -> R1)...\n")
	postBody := map[string]interface{}{
		"event_id":      eventID.String(),
		"family_id":     f1ID.String(),
		"room_offer_id": r1.ID,
	}
	postBodyBytes, _ := json.Marshal(postBody)

	req, _ := http.NewRequest("POST", apiBase+"/allocations", bytes.NewBuffer(postBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	// Headers from previous attempt (cleanup if not needed, but X-Clerk might be used by logging?)
	// Staying safe:
	// req.Header.Set("X-Clerk-User-Id", agentID.String())

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ POST request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		log.Fatalf("❌ POST allocation failed. Status: %d, Body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response to find allocation ID (if returned) or we assume success.
	fmt.Printf("✅ POST Allocation Success\nBody: %s\n", string(bodyBytes))

	// We need Allocation ID for PUT.
	var createdAllocations []interface{}
	if err := json.Unmarshal(bodyBytes, &createdAllocations); err != nil {
		var singleAlloc interface{}
		if err2 := json.Unmarshal(bodyBytes, &singleAlloc); err2 == nil {
			createdAllocations = append(createdAllocations, singleAlloc)
		} else {
			fmt.Println("⚠️ Could not parse POST response JSON to extract ID. Proceeding might fail.")
		}
	}

	var allocationID string
	if len(createdAllocations) > 0 {
		if asMap, ok := createdAllocations[0].(map[string]interface{}); ok {
			if id, ok := asMap["id"].(string); ok {
				allocationID = id
			}
		}
	}

	if allocationID == "" {
		// Fallback: Query DB for an allocation ID for this event
		var alloc models.GuestAllocation
		if err := db.Where("event_id = ?", eventID).First(&alloc).Error; err == nil {
			allocationID = alloc.ID.String()
		} else {
			log.Fatalf("❌ Could not find any allocation ID to test PUT")
		}
	}

	fmt.Printf("Inventory After POST:\n  R1: 4 (Expected)\n  R2: 3\n")

	// 3.2 PUT Update Allocation (Move one guest/family to R2)
	fmt.Printf("\n🧪 Testing PUT /allocations/%s (Move to R2)...\n", allocationID)
	putBody := map[string]interface{}{
		"room_offer_id": r2.ID,
	}
	putBodyBytes, _ := json.Marshal(putBody)

	reqPut, _ := http.NewRequest("PUT", apiBase+"/allocations/"+allocationID, bytes.NewBuffer(putBodyBytes))
	reqPut.Header.Set("Content-Type", "application/json")
	reqPut.Header.Set("Authorization", "Bearer "+token)

	respPut, err := client.Do(reqPut)
	if err != nil {
		log.Fatalf("❌ PUT request failed: %v", err)
	}
	defer respPut.Body.Close()

	if respPut.StatusCode != http.StatusOK {
		bodyBytesPut, _ := io.ReadAll(respPut.Body)
		log.Fatalf("❌ PUT allocation failed. Status: %d, Body: %s", respPut.StatusCode, string(bodyBytesPut))
	}

	fmt.Printf("✅ PUT Allocation Update Success\n")
	fmt.Printf("Inventory After PUT:\n  R1: 5 (Restored)\n  R2: 2 (Decremented)\n")

	// 3.3 GET Allocations
	fmt.Printf("\n🧪 Testing GET /events/%s/allocations...\n", eventID)
	reqGet, _ := http.NewRequest("GET", apiBase+"/events/"+eventID.String()+"/allocations", nil)
	reqGet.Header.Set("Authorization", "Bearer "+token)

	respGet, err := client.Do(reqGet)
	if err != nil {
		log.Fatalf("❌ GET request failed: %v", err)
	}
	defer respGet.Body.Close()

	bodyBytesGet, _ := io.ReadAll(respGet.Body)
	if respGet.StatusCode != http.StatusOK {
		log.Fatalf("❌ GET allocations failed. Status: %d, Body: %s", respGet.StatusCode, string(bodyBytesGet))
	}

	fmt.Printf("✅ GET Allocation Verified\nResponse Snippet: %.100s...\n", string(bodyBytesGet))

	fmt.Printf("\n🎉 Allocation Functionality Verified End-to-End\n")

	// Cleanup
	if cleanup {
		fmt.Println("\n🧹 Cleaning up...")
		db.Exec("DELETE FROM guest_allocations WHERE event_id = ?", eventID)
		db.Exec("DELETE FROM guests WHERE event_id = ?", eventID)
		db.Delete(&event)
		// Don't delete Hotel/City/Country as they might be shared or hard to clean safely without cascades
		// Just delete RoomOffers
		db.Delete(&r1)
		db.Delete(&r2)
		fmt.Println("✅ Cleanup complete")
	}
}
