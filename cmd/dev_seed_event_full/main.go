package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/config"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/datatypes"
)

// RoomsInventoryItem struct to match JSON structure
type RoomsInventoryItem struct {
	RoomOfferID    string `json:"room_offer_id"`
	RoomType       string `json:"room_type"`
	Capacity       int    `json:"capacity"`
	TotalRooms     int    `json:"total_rooms"`
	AvailableRooms int    `json:"available_rooms"`
}

func main() {
	fmt.Println("⚠️  WARNING: RUNNING IN DEV MODE. THIS SCRIPT MODIFIES DATA.")
	time.Sleep(2 * time.Second)

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize DB
	config.Load()
	store.InitDB()
	db := store.DB

	targetEventID := "b5390600-f68a-47c5-b399-877e61f14be2"
	eventUUID, err := uuid.Parse(targetEventID)
	if err != nil {
		log.Fatalf("Invalid UUID: %v", err)
	}

	var event models.Event
	if err := db.First(&event, "id = ?", eventUUID).Error; err != nil {
		log.Fatalf("Event not found: %v", err)
	}

	fmt.Printf("✅ Found Event: %s\n", event.ID)

	// 1. Fix Inventory
	// Need a valid RoomOfferID. We'll create a dummy RoomOffer or find one.
	// Logic: Find a RoomOffer for this Hotel if exists, else create one.
	// But first, let's fix the Hotel ID.

	// 1.5 Ensure Country and City exist
	country := models.Country{Code: "US", Name: "United States", PhoneCode: "+1"}
	if err := db.FirstOrCreate(&country, models.Country{Code: "US"}).Error; err != nil {
		log.Printf("⚠️ Failed to ensure country: %v", err)
	}

	city := models.City{ID: "NYC", CountryCode: "US", Name: "New York", IsPopular: true}
	if err := db.FirstOrCreate(&city, models.City{ID: "NYC"}).Error; err != nil {
		log.Printf("⚠️ Failed to ensure city: %v", err)
	}

	// 2. Create/Fix Hotel
	if event.HotelID == "default" || event.HotelID == "" {
		newHotel := models.Hotel{
			ID:         fmt.Sprintf("H-%d", rand.Intn(100000)), // Random Hotel Code
			CityID:     "NYC",
			Name:       "Grand TBO Hotel",
			StarRating: 5,
			Address:    "123 TBO Street",
			Occupancy:  500,
			Facilities: datatypes.JSON("[]"),
			ImageUrls:  datatypes.JSON("[]"),
		}
		// Upsert logic for Hotel (if ID collision, just update)
		if err := db.Save(&newHotel).Error; err != nil {
			log.Fatalf("Failed to create hotel: %v", err)
		}
		event.HotelID = newHotel.ID
		if err := db.Save(&event).Error; err != nil {
			log.Fatalf("Failed to update event hotel: %v", err)
		}
		fmt.Printf("✅ Updated Event Hotel ID: %s\n", event.HotelID)
	} else {
		fmt.Printf("ℹ️ Event Hotel ID already set: %s\n", event.HotelID)
	}

	// Now Create RoomOffer linked to this Hotel
	var roomOffer models.RoomOffer
	// Check if any room offer exists for this hotel
	if err := db.Where("hotel_id = ?", event.HotelID).First(&roomOffer).Error; err != nil {
		// Create new RoomOffer
		roomOffer = models.RoomOffer{
			ID:             uuid.New().String(),
			HotelID:        event.HotelID,
			Name:           "Deluxe Suite",
			BookingCode:    "DLX",
			MaxCapacity:    4,
			TotalFare:      150.00,
			Count:          20,
			CancelPolicies: datatypes.JSON("[]"),
		}
		if err := db.Create(&roomOffer).Error; err != nil {
			log.Fatalf("Failed to create RoomOffer: %v", err)
		}
		fmt.Printf("✅ Created New Room Offer: %s\n", roomOffer.ID)
	} else {
		fmt.Printf("ℹ️ Using Existing Room Offer: %s\n", roomOffer.ID)
	}

	// Update Inventory JSON
	inventory := []RoomsInventoryItem{
		{
			RoomOfferID:    roomOffer.ID,
			RoomType:       roomOffer.Name,
			Capacity:       roomOffer.MaxCapacity,
			TotalRooms:     10,
			AvailableRooms: 10,
		},
	}
	inventoryJSON, _ := json.Marshal(inventory)
	event.RoomsInventory = datatypes.JSON(inventoryJSON)
	if err := db.Save(&event).Error; err != nil {
		log.Fatalf("Failed to update rooms_inventory: %v", err)
	}
	fmt.Println("✅ Updated Event Rooms Inventory")

	// 3. Head Guest
	var headGuest models.Guest
	if event.HeadGuestID != uuid.Nil {
		if err := db.First(&headGuest, "id = ?", event.HeadGuestID).Error; err == nil {
			fmt.Printf("ℹ️ Head Guest already exists: %s\n", headGuest.ID)
		} else {
			// ID exists but not in DB? Reset.
			event.HeadGuestID = uuid.Nil
		}
	}

	if event.HeadGuestID == uuid.Nil {
		headGuestID := uuid.New()
		familyName := fmt.Sprintf("HeadFamily-%d", rand.Intn(1000))
		headGuest = models.Guest{
			ID:          headGuestID,
			Name:        "Head Guest " + familyName,
			Age:         35,
			Type:        "Adult", // Helper function sets this?
			Phone:       "555-0100",
			Email:       fmt.Sprintf("headguest-%d@example.com", rand.Intn(10000)),
			EventID:     event.ID,
			FamilyID:    uuid.New(), // Its own family
			ArrivalDate: time.Now(),
		}
		if err := db.Create(&headGuest).Error; err != nil {
			log.Fatalf("Failed to create head guest: %v", err)
		}
		event.HeadGuestID = headGuest.ID
		if err := db.Save(&event).Error; err != nil {
			log.Fatalf("Failed to update event head guest: %v", err)
		}
		fmt.Printf("✅ Created Head Guest: %s\n", headGuest.ID)
	}

	// 4. Create 100 Guests (25 Families)
	totalGuests := 100
	guestsPerFamily := 4
	totalFamilies := totalGuests / guestsPerFamily
	createdGuests := 0
	createdFamilies := 0

	for i := 0; i < totalFamilies; i++ {
		familyID := uuid.New()
		familyName := fmt.Sprintf("Family-%d-%d", i, rand.Intn(1000))
		for j := 0; j < guestsPerFamily; j++ {
			guest := models.Guest{
				ID:       uuid.New(),
				Name:     fmt.Sprintf("%s Member %d", familyName, j+1),
				Age:      20 + j*5, // Simple age logic
				Type:     "Adult",
				Phone:    fmt.Sprintf("555-01%02d", createdGuests%100),
				Email:    fmt.Sprintf("guest-%d-%d@example.com", i, j),
				EventID:  event.ID,
				FamilyID: familyID,
			}
			if j == 3 { // Make one a child
				guest.Age = 8
				guest.Type = "Child"
			}

			if err := db.Create(&guest).Error; err != nil {
				log.Printf("❌ Failed to create guest: %v", err)
			} else {
				createdGuests++
			}
		}
		createdFamilies++
	}

	fmt.Println("--------------------------------------------------")
	fmt.Printf("🎉 Seed Complete!\n")
	fmt.Printf("Event ID: %s\n", event.ID)
	fmt.Printf("Hotel ID: %s\n", event.HotelID)
	fmt.Printf("Head Guest ID: %s\n", event.HeadGuestID)
	fmt.Printf("Total Guests Created: %d\n", createdGuests)
	fmt.Printf("Total Families Created: %d\n", createdFamilies)
	fmt.Printf("Inventory JSON: %s\n", string(inventoryJSON))
	fmt.Println("--------------------------------------------------")
}
