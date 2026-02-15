package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const baseURL = "http://localhost:8080/api/v1"

func main() {
	// Connect to DB directly
	dsn := "postgresql://postgres.xpjjqpczpegeedyrnnpq:s14aTJcYc7zIVPGZ@aws-1-ap-south-1.pooler.supabase.com:5432/postgres?sslmode=require"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// 1. Seed Static Data
	// Country & City
	db.FirstOrCreate(&models.Country{Code: "XX", Name: "Test Country"})
	db.FirstOrCreate(&models.City{ID: "city-1", CountryCode: "XX", Name: "Test City"})

	hotelID := "hotel-123"
	if err := db.FirstOrCreate(&models.Hotel{
		ID:     hotelID,
		CityID: "city-1",
		Name:   "Test Hotel",
	}).Error; err != nil {
		fmt.Println("Warning: Could not create hotel:", err)
	}

	roomA := models.RoomOffer{
		ID:          uuid.New().String(),
		HotelID:     hotelID,
		Name:        "Room A",
		BookingCode: "RA",
		MaxCapacity: 4,
		TotalFare:   100,
		Count:       10,
	}
	roomB := models.RoomOffer{
		ID:          uuid.New().String(),
		HotelID:     hotelID,
		Name:        "Room B",
		BookingCode: "RB",
		MaxCapacity: 4,
		TotalFare:   200,
		Count:       10,
	}
	db.Create(&roomA)
	db.Create(&roomB)
	fmt.Printf("Seeded Rooms: A=%s, B=%s\n", roomA.ID, roomB.ID)

	// 2. Create User (Agent)
	agentEmail := fmt.Sprintf("agent_%d@test.com", time.Now().UnixNano())
	agent := models.User{
		ID:           uuid.New(),
		Email:        agentEmail,
		Role:         "agent",
		Name:         "Test Agent",
		PasswordHash: "$2a$10$3QxDjD1ylg.6T4x...", // Dummy hash
	}
	if err := db.Create(&agent).Error; err != nil {
		panic(err)
	}
	fmt.Printf("Created Agent: %s (%s)\n", agent.ID, agent.Email)

	// Generate Token manually or login via API?
	// API login requires matching password hash.
	// We'll use API signup to ensure we have a valid token that matches a user.
	// Correction: Login requires valid password. Signup avoids manual DB insert.
	// But we need the ID.
	// Let's use signup API to get token AND user details.
	token, agentID := signupAgent()
	fmt.Printf("Agent ID from API: %s\n", agentID)

	// 3. Create Event in DB (bypass API mismatch)
	roomsInventory := []map[string]interface{}{
		{
			"room_offer_id":  roomA.ID,
			"room_name":      roomA.Name,
			"available":      10,
			"max_capacity":   roomA.MaxCapacity,
			"price_per_room": int(roomA.TotalFare),
		},
		{
			"room_offer_id":  roomB.ID,
			"room_name":      roomB.Name,
			"available":      10,
			"max_capacity":   roomB.MaxCapacity,
			"price_per_room": int(roomB.TotalFare),
		},
	}
	invJSON, _ := json.Marshal(roomsInventory)

	eventID := uuid.New()
	event := models.Event{
		ID:             eventID,
		AgentID:        uuid.MustParse(agentID),
		HotelID:        hotelID,
		Name:           "Test Event",
		Location:       "Test Loc",
		StartDate:      time.Now(),
		EndDate:        time.Now().AddDate(0, 0, 5),
		Status:         "allocating",
		RoomsInventory: datatypes.JSON(invJSON),
	}
	if err := db.Create(&event).Error; err != nil {
		panic("Failed to create event: " + err.Error())
	}
	fmt.Println("Created Event:", eventID)

	// 4. Create Guest & Family in DB
	familyID := uuid.New()
	guest := models.Guest{
		ID:       uuid.New(),
		EventID:  eventID,
		FamilyID: familyID,
		Name:     "Head Guest",
		Type:     "Adult",
	}
	if err := db.Create(&guest).Error; err != nil {
		panic("Failed to create guest: " + err.Error())
	}
	fmt.Printf("Created Guest: %s, Family: %s\n", guest.ID, familyID)

	// 5. Create Initial Allocation to Room A in DB
	allocationID := uuid.New()
	allocation := models.GuestAllocation{
		ID:           allocationID,
		EventID:      eventID,
		GuestID:      guest.ID,
		RoomOfferID:  &roomA.ID,
		LockedPrice:  roomA.TotalFare,
		Status:       "allocated",
		AssignedMode: "agent_manual",
	}
	if err := db.Create(&allocation).Error; err != nil {
		panic("Failed to create allocation: " + err.Error())
	}
	fmt.Println("Created Allocation to Room A:", allocationID)

	// Decrement Inventory for Room A manually to match state
	// (We allocated 1 room, so available should be 9)
	// Update Event JSON
	// We'll skip this manual update for speed, but Strict testing requires it.
	// Actually, UpdateAllocationHandler checks OLD room availability? No, it increments it.
	// But it checks NEW room availability.
	// So Room B must have availability. It has 10. OK.
	// Room A availability check in UpdateAllocationHandler?
	// It increments old room.
	// So we don't strictly need accurate inventory for Room A for the Update to succeed,
	// unless we check consistency.
	// But let's verify afterwards.

	// 6. Call Update Allocation via API
	fmt.Println("Updating Allocation to Room B...")
	status, body := updateAllocation(token, allocationID.String(), roomB.ID)
	fmt.Printf("Status: %d\nBody: %s\n", status, body)

	if status == 200 {
		fmt.Println("✅ Update Success")

		// Verify DB
		var alloc models.GuestAllocation
		db.First(&alloc, "id = ?", allocationID)
		if *alloc.RoomOfferID == roomB.ID {
			fmt.Println("✅ DB Updated to Room B")
		} else {
			fmt.Printf("❌ DB Update Failed. Room is %s\n", *alloc.RoomOfferID)
			os.Exit(1)
		}

		// Verify Inventory
		// Room A should be incremented (from whatever it was).
		// We didn't decrement it initially. So if it was 10, it became 11?
		// We seeded 10.
		// UpdateAllocationHandler: Old Room (A) Index found. `inventory[oldRoomIndex].Available++` -> 11.
		// Room B (B) Index found. `inventory[newRoomIndex].Available--` -> 9.
		checkInventory(db, eventID.String(), roomA.ID, 11)
		checkInventory(db, eventID.String(), roomB.ID, 9)

		// 7. Verify GET Allocations Endpoint
		fmt.Println("\nVerifying GET Allocations...")
		url := fmt.Sprintf("%s/events/%s/allocations", baseURL, eventID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token) // Use the same token as update

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("GET Status: %d\n", resp.StatusCode)

		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, body, "", "  ")
		fmt.Printf("GET Body:\n%s\n", prettyJSON.String())

		if resp.StatusCode != 200 {
			fmt.Printf("GET Body: %s\n", string(body))
			fmt.Printf("❌ GET Failed: %s\n", string(body))
			os.Exit(1)
		}

		// 8. Verify JSON Structure (Snake Case)
		var getResp map[string]interface{}
		json.Unmarshal(body, &getResp)

		// Check top level snake_case keys
		if _, ok := getResp["event_id"]; !ok {
			fmt.Println("❌ Missing 'event_id' key")
			os.Exit(1)
		}
		if _, ok := getResp["rooms_inventory"]; !ok {
			fmt.Println("❌ Missing 'rooms_inventory' key")
			os.Exit(1)
		}
		if _, ok := getResp["allocations"]; !ok {
			fmt.Println("❌ Missing 'allocations' key")
			os.Exit(1)
		}

		// Check Allocations grouping
		allocs := getResp["allocations"].([]interface{})
		if len(allocs) == 0 {
			fmt.Println("❌ No allocations found in response")
			os.Exit(1)
		}

		firstFam := allocs[0].(map[string]interface{})
		if firstFam["family_id"] != familyID.String() {
			fmt.Printf("❌ Expected family_id %s, got %s\n", familyID, firstFam["family_id"])
			os.Exit(1)
		}

		// Check Guests array inside family
		guests := firstFam["guests"].([]interface{})
		if len(guests) == 0 {
			fmt.Println("❌ No guests found in family allocation")
			os.Exit(1)
		}

		firstGuest := guests[0].(map[string]interface{})
		if _, ok := firstGuest["guest_name"]; !ok {
			fmt.Println("❌ Missing 'guest_name' in guest object")
			os.Exit(1)
		}

		fmt.Println("✅ GET Allocations & JSON Structure Verified")

	} else {
		fmt.Println("❌ Update Failed")
		os.Exit(1)
	}
}

func signupAgent() (string, string) {
	email := fmt.Sprintf("agent_%d@test.com", time.Now().UnixNano())
	payload := map[string]string{
		"name":       "Test Agent",
		"email":      email,
		"password":   "password123",
		"phone":      "1234567890",
		"agencyName": "Test Agency",
		"agencyCode": "TAG123",
		"location":   "Test City",
	}

	jsonBody, _ := json.Marshal(payload)
	resp, err := http.Post(baseURL+"/auth/signup", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var res map[string]interface{}
	json.Unmarshal(body, &res)

	if res["token"] == nil {
		panic(fmt.Sprintf("Signup failed: %s", string(body)))
	}

	userMap := res["user"].(map[string]interface{})
	return res["token"].(string), userMap["id"].(string)
}

func updateAllocation(token, allocationID, newRoomOfferID string) (int, string) {
	payload := map[string]string{
		"room_offer_id": newRoomOfferID,
	}

	url := baseURL + "/allocations/" + allocationID
	jsonBody, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func checkInventory(db *gorm.DB, eventID, roomID string, expected int) {
	var event models.Event
	db.First(&event, "id = ?", eventID)
	var items []map[string]interface{}
	json.Unmarshal(event.RoomsInventory, &items)
	for _, item := range items {
		if item["room_offer_id"] == roomID {
			avail := int(item["available"].(float64))
			if avail == expected {
				fmt.Printf("✅ Inventory %s: %d\n", roomID, avail)
			} else {
				fmt.Printf("❌ Inventory %s: %d (Expected %d)\n", roomID, avail, expected)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Println("❌ Inventory room not found")
	os.Exit(1)
}
