package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
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
)

const (
	TestAgentEmail    = "test_agent@tbo.com"
	TestAgentPassword = "password123"
)

func main() {
	fmt.Println("🚀 Starting Guest Allocation Test Script (50 Guests, 50 Rooms, NO Allocations)")

	// 1. Setup & DB Connection
	if err := godotenv.Load("../../.env"); err != nil {
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

	// 2. Create/Get Persistent Agent
	var agent models.User
	if err := db.Where("email = ?", TestAgentEmail).First(&agent).Error; err != nil {
		// Create new agent
		fmt.Printf("ℹ️ Creating new test agent...\n")
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(TestAgentPassword), bcrypt.DefaultCost)
		agent = models.User{
			ID:           uuid.New(),
			Email:        TestAgentEmail,
			PasswordHash: string(hashedPassword),
			Role:         "agent",
			Name:         "Test Agent",
			Phone:        "1234567890",
		}
		if err := db.Create(&agent).Error; err != nil {
			log.Fatalf("❌ Failed to create agent: %v", err)
		}
		// Create profile
		profile := models.AgentProfile{
			UserID:     agent.ID,
			AgencyName: "Test Agency",
			AgencyCode: "TEST001",
		}
		db.Create(&profile)
	} else {
		// Reset password ensures it matches what we tell the user
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(TestAgentPassword), bcrypt.DefaultCost)
		agent.PasswordHash = string(hashedPassword)
		db.Save(&agent)
		fmt.Printf("ℹ️ Using existing agent: %s\n", agent.ID)
	}

	// 3. Create Hotel & Event
	hotelID := "TEST_HOTEL_ALLOCATION"
	// Ensure Hotel exists
	var hotel models.Hotel
	if err := db.FirstOrCreate(&hotel, models.Hotel{
		ID:         hotelID,
		CityID:     "NYC",
		Name:       "Allocation Test Hotel",
		StarRating: 5,
		Occupancy:  100,
	}).Error; err != nil {
		log.Printf("⚠️ Warning: Hotel creation might have failed: %v", err)
	}

	eventID := uuid.New()
	headGuestID := uuid.New()

	// 4. Create Room Offers (50 Rooms Total)
	// Type A: Standard (Cap 2) - 20 rooms
	// Type B: Deluxe (Cap 4) - 20 rooms
	// Type C: Suite (Cap 6) - 10 rooms
	roomDefs := []struct {
		Name  string
		Code  string
		Cap   int
		Count int
		Fare  float64
		ID    string
	}{
		{"Standard", "STD", 2, 20, 100.0, uuid.New().String()},
		{"Deluxe", "DLX", 4, 20, 200.0, uuid.New().String()},
		{"Suite", "STE", 6, 10, 500.0, uuid.New().String()},
	}

	var inventory []InventoryItem

	for _, rd := range roomDefs {
		offer := models.RoomOffer{
			ID:          rd.ID,
			HotelID:     hotelID,
			Name:        rd.Name,
			BookingCode: rd.Code,
			MaxCapacity: rd.Cap,
			TotalFare:   rd.Fare,
			Count:       rd.Count,
		}
		if err := db.Create(&offer).Error; err != nil {
			log.Fatalf("❌ Failed to create room offer %s: %v", rd.Name, err)
		}
		inventory = append(inventory, InventoryItem{
			RoomOfferID: rd.ID,
			RoomName:    rd.Name,
			Capacity:    rd.Cap,
			Total:       rd.Count,
			Available:   rd.Count,
		})
	}
	fmt.Println("✅ Created 3 Room Types (Total 50 Rooms)")

	// Create Event
	invJSON, _ := json.Marshal(inventory)
	event := models.Event{
		ID:             eventID,
		AgentID:        agent.ID,
		HeadGuestID:    headGuestID,
		HotelID:        hotelID,
		Name:           "50 Guests NO Allocation Test",
		Status:         "allocating",
		RoomsInventory: datatypes.JSON(invJSON),
		StartDate:      time.Now(),
		EndDate:        time.Now().Add(48 * time.Hour),
	}
	if err := db.Create(&event).Error; err != nil {
		log.Fatalf("❌ Failed to create Event: %v", err)
	}

	// 5. Create 50 Guests (Families)
	// Create 10 families of 4 (40 guests)
	for i := 0; i < 10; i++ {
		famID := uuid.New()
		createFamily(db, eventID, famID, 4, fmt.Sprintf("Fam4-%d", i))
	}

	// Create 5 families of 2 (10 guests)
	for i := 0; i < 5; i++ {
		famID := uuid.New()
		createFamily(db, eventID, famID, 2, fmt.Sprintf("Fam2-%d", i))
	}
	fmt.Println("✅ Created 50 Guests across 15 Families")

	// 6. SKIP Allocations (as requested)
	fmt.Println("\nℹ️  Skipping automatic allocation as requested.")
	fmt.Println("   You can now use the frontend to test manual allocation.")

	// 7. Verification Results
	fmt.Println("\n==================================================")
	fmt.Println("🎉  TEST SETUP COMPLETE (DATA ONLY)")
	fmt.Println("==================================================")
	fmt.Printf("✅  Event Created: %s\n", eventID)
	fmt.Printf("✅  Agent Email:   %s\n", TestAgentEmail)
	fmt.Printf("✅  Agent Pass:    %s\n", TestAgentPassword)
	fmt.Printf("✅  Guests:        50 (15 Families)\n")
	fmt.Printf("✅  Rooms:         50 (20 STD, 20 DLX, 10 STE)\n")
	fmt.Println("==================================================")
}

func createFamily(db *gorm.DB, eventID, familyID uuid.UUID, size int, baseName string) {
	for i := 0; i < size; i++ {
		guest := models.Guest{
			ID:       uuid.New(),
			Name:     fmt.Sprintf("%s-M%d", baseName, i+1),
			Age:      25 + i,
			Type:     "Adult",
			EventID:  eventID,
			FamilyID: familyID,
		}
		if err := db.Create(&guest).Error; err != nil {
			log.Fatalf("Failed create guest: %v", err)
		}
	}
}
