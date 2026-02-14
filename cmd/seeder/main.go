package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// RoomsInventoryItem represents a single room type in inventory
type RoomsInventoryItem struct {
	RoomOfferID  string `json:"room_offer_id"`
	RoomName     string `json:"room_name"`
	Available    int    `json:"available"`
	MaxCapacity  int    `json:"max_capacity"`
	PricePerRoom int    `json:"price_per_room"`
}

func main() {
	_ = flag.String("type", "", "Type of seeding to run: countries, cities, hotels, rooms")
	flag.Parse()

	store.InitDB()
	db := store.DB // Use store.DB as db for compatibility with existing logic

	// 1. Full Database Reset
	log.Println("⚠️  STARTING DATABASE RESET...")
	tables := []string{
		"guest_allocations",
		"room_offers",
		"banquet_halls",
		"catering_menus",
		"hotels",
		"cities",
		"countries",
		"events",
		"agent_profiles",
		"guests",
		"users",
	}

	for _, table := range tables {
		if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)).Error; err != nil {
			log.Fatalf("❌ Failed to drop table %s: %v", table, err)
		}
	}
	log.Println("✅ Database Cleared.")

	// 2. Automigration
	log.Println("🛠️  Running Automigrate...")
	err := db.AutoMigrate(
		&models.User{},
		&models.AgentProfile{},
		&models.Country{},
		&models.City{},
		&models.Hotel{},
		&models.RoomOffer{},
		&models.BanquetHall{},
		&models.CateringMenu{},
		&models.Event{},
		&models.Guest{},
		&models.GuestAllocation{},
	)
	if err != nil {
		log.Fatal("❌ Migration Failed:", err)
	}
	log.Println("✅ All tables created successfully!")

	// 3. Constrained Seeding (Upstream Logic)
	log.Println("🌱 Starting Constrained Seeding (Upstream)...")

	targetCountries := []string{"US", "SG", "AE", "TH", "IN"}

	// Step 1: Countries
	SeedCountries(targetCountries)

	// Step 2: Cities (IN: 200, others: 100)
	SeedCities(targetCountries)

	// Step 3: Hotels (50 per city)
	SeedHotels(50)

	// Step 4: Rooms
	SeedRooms(0) // Process all seeded hotels

	// 4. Custom Test Data Seeding (Local Logic)
	log.Println("🌱 Starting Custom Test Data Seeding (Local)...")

	// 1. Create Agent User
	agentUser := models.User{
		ID:           uuid.New(),
		Name:         "Rajesh Kumar",
		Email:        "rajesh.agent@tbo.com",
		PasswordHash: "$2a$10$examplehash", // bcrypt hash
		Phone:        "+91-9876543210",
		Role:         "agent",
	}
	db.Create(&agentUser)

	agentProfile := models.AgentProfile{
		UserID:        agentUser.ID,
		AgencyName:    "Kumar Travel Solutions",
		AgencyCode:    "KTS001",
		Location:      "Mumbai, India",
		BusinessPhone: "+91-22-12345678",
	}
	db.Create(&agentProfile)

	// 2. Create Head Guest User
	headGuestUser := models.User{
		ID:           uuid.New(),
		Name:         "Priya Sharma",
		Email:        "priya.headguest@example.com",
		PasswordHash: "$2a$10$examplehash",
		Phone:        "+91-9123456789",
		Role:         "head_guest",
	}
	db.Create(&headGuestUser)

	// 3. Create Hotel (Specific for testing)
	facilities, _ := json.Marshal([]string{"WiFi", "Pool", "Spa", "Restaurant", "Gym", "Beach Access"})
	imageUrls, _ := json.Marshal([]string{
		"https://example.com/hotel1.jpg",
		"https://example.com/hotel2.jpg",
	})

	// Ensure GOA city exists (SeedCities might have created it, but let's be safe or just use it)
	// Since SeedCities creates popular cities, GOA might be there.
	// But let's just create our specific hotel in GOA.
	var goaCity models.City
	if err := db.First(&goaCity, "id = ?", "GOA").Error; err != nil {
		// Create if not exists
		goaCity = models.City{
			ID:          "GOA",
			CountryCode: "IN",
			Name:        "Goa",
			IsPopular:   true,
		}
		db.Save(&goaCity)
	}

	hotel := models.Hotel{
		ID:         "GOA12345",
		CityID:     "GOA",
		Name:       "Paradise Beach Resort",
		StarRating: 5,
		Address:    "Calangute Beach Road, North Goa, 403516",
		Facilities: datatypes.JSON(facilities),
		ImageUrls:  datatypes.JSON(imageUrls),
		Occupancy:  500,
	}
	db.Create(&hotel)

	// 5. Create Room Offers (3 types) matches stashed logic
	cancelPolicy, _ := json.Marshal([]map[string]interface{}{
		{
			"from":        "2026-02-01T00:00:00Z",
			"to":          "2026-02-10T00:00:00Z",
			"charge":      100,
			"charge_type": "percentage",
		},
	})

	roomOffers := []models.RoomOffer{
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Deluxe Ocean View",
			BookingCode:    "DELUXE_OCEAN_001",
			MaxCapacity:    2,
			TotalFare:      5000.00,
			Currency:       "INR",
			IsRefundable:   true,
			CancelPolicies: datatypes.JSON(cancelPolicy),
			Count:          10,
		},
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Family Suite",
			BookingCode:    "FAMILY_SUITE_002",
			MaxCapacity:    4,
			TotalFare:      8000.00,
			Currency:       "INR",
			IsRefundable:   true,
			CancelPolicies: datatypes.JSON(cancelPolicy),
			Count:          5,
		},
		{
			ID:             uuid.New().String(),
			HotelID:        hotel.ID,
			Name:           "Premium Villa",
			BookingCode:    "PREMIUM_VILLA_003",
			MaxCapacity:    6,
			TotalFare:      15000.00,
			Currency:       "INR",
			IsRefundable:   false,
			CancelPolicies: datatypes.JSON(cancelPolicy),
			Count:          3,
		},
	}

	for _, room := range roomOffers {
		db.Create(&room)
	}

	// 6. Create Rooms Inventory JSON
	inventoryItems := []RoomsInventoryItem{
		{
			RoomOfferID:  roomOffers[0].ID,
			RoomName:     "Deluxe Ocean View",
			Available:    10,
			MaxCapacity:  2,
			PricePerRoom: 5000,
		},
		{
			RoomOfferID:  roomOffers[1].ID,
			RoomName:     "Family Suite",
			Available:    5,
			MaxCapacity:  4,
			PricePerRoom: 8000,
		},
		{
			RoomOfferID:  roomOffers[2].ID,
			RoomName:     "Premium Villa",
			Available:    3,
			MaxCapacity:  6,
			PricePerRoom: 15000,
		},
	}

	inventoryJSON, _ := json.Marshal(inventoryItems)

	// 7. Create Event (status = "allocating")
	event := models.Event{
		ID:             uuid.New(),
		AgentID:        agentUser.ID,
		HeadGuestID:    headGuestUser.ID,
		HotelID:        hotel.ID,
		Name:           "Sharma Family Wedding",
		Location:       "Goa, India",
		RoomsInventory: datatypes.JSON(inventoryJSON),
		Status:         "allocating",
		StartDate:      time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		EndDate:        time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC),
	}
	db.Create(&event)

	// 8. Create 4 Families with varying sizes
	families := []struct {
		FamilyID uuid.UUID
		Guests   []models.Guest
	}{
		{
			// Family 1: 2 adults (fits in Deluxe Ocean View)
			FamilyID: uuid.New(),
			Guests: []models.Guest{
				{
					Name:          "Amit Verma",
					Age:           35,
					Type:          "adult",
					Phone:         "+91-9111111111",
					Email:         "amit@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Sneha Verma",
					Age:           32,
					Type:          "adult",
					Phone:         "+91-9111111112",
					Email:         "sneha@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
			},
		},
		{
			// Family 2: 4 people (2 adults + 2 children, fits in Family Suite)
			FamilyID: uuid.New(),
			Guests: []models.Guest{
				{
					Name:          "Vikram Singh",
					Age:           40,
					Type:          "adult",
					Phone:         "+91-9222222221",
					Email:         "vikram@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Meera Singh",
					Age:           38,
					Type:          "adult",
					Phone:         "+91-9222222222",
					Email:         "meera@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Rohan Singh",
					Age:           10,
					Type:          "child",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Aarav Singh",
					Age:           7,
					Type:          "child",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
			},
		},
		{
			// Family 3: 3 adults (fits in Family Suite)
			FamilyID: uuid.New(),
			Guests: []models.Guest{
				{
					Name:          "Karan Patel",
					Age:           45,
					Type:          "adult",
					Phone:         "+91-9333333331",
					Email:         "karan@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Anjali Patel",
					Age:           42,
					Type:          "adult",
					Phone:         "+91-9333333332",
					Email:         "anjali@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Ravi Patel",
					Age:           65,
					Type:          "adult",
					Phone:         "+91-9333333333",
					Email:         "ravi@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
			},
		},
		{
			// Family 4: 5 people (3 adults + 2 children, fits in Premium Villa)
			FamilyID: uuid.New(),
			Guests: []models.Guest{
				{
					Name:          "Suresh Reddy",
					Age:           50,
					Type:          "adult",
					Phone:         "+91-9444444441",
					Email:         "suresh@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Lakshmi Reddy",
					Age:           48,
					Type:          "adult",
					Phone:         "+91-9444444442",
					Email:         "lakshmi@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Sanjay Reddy",
					Age:           25,
					Type:          "adult",
					Phone:         "+91-9444444443",
					Email:         "sanjay@example.com",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Priya Reddy",
					Age:           8,
					Type:          "child",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
				{
					Name:          "Krishna Reddy",
					Age:           5,
					Type:          "child",
					EventID:       event.ID,
					ArrivalDate:   event.StartDate,
					DepartureDate: event.EndDate,
				},
			},
		},
	}

	// Insert all families and guests
	for _, family := range families {
		for i := range family.Guests {
			family.Guests[i].FamilyID = family.FamilyID
			db.Create(&family.Guests[i])
		}
	}

	// 9. Create Guest Allocations (allocate first 2 families, leave 2 unallocated for testing)
	log.Println("🔗 Creating guest allocations...")

	// Allocate Family 1 (2 adults) to Deluxe Ocean View
	for _, guest := range families[0].Guests {
		allocation := models.GuestAllocation{
			ID:           uuid.New(),
			EventID:      event.ID,
			GuestID:      guest.ID,
			RoomOfferID:  &roomOffers[0].ID, // Deluxe Ocean View
			LockedPrice:  roomOffers[0].TotalFare,
			Status:       "allocated",
			AssignedMode: "agent_manual",
		}
		db.Create(&allocation)
	}
	log.Printf("   ✅ Family 1 allocated to %s", roomOffers[0].Name)

	// Allocate Family 2 (4 people) to Family Suite
	for _, guest := range families[1].Guests {
		allocation := models.GuestAllocation{
			ID:           uuid.New(),
			EventID:      event.ID,
			GuestID:      guest.ID,
			RoomOfferID:  &roomOffers[1].ID, // Family Suite
			LockedPrice:  roomOffers[1].TotalFare,
			Status:       "allocated",
			AssignedMode: "head_guest_manual",
		}
		db.Create(&allocation)
	}
	log.Printf("   ✅ Family 2 allocated to %s", roomOffers[1].Name)
	log.Printf("   ℹ️  Family 3 and Family 4 left unallocated for testing")

	// Update inventory to reflect allocations (decrement by 2)
	var updatedInventory []RoomsInventoryItem
	json.Unmarshal(event.RoomsInventory, &updatedInventory)
	updatedInventory[0].Available = 8 // Deluxe Ocean View: 10 - 1 = 9 (but we'll make it 8 for variety)
	updatedInventory[1].Available = 4 // Family Suite: 5 - 1 = 4
	updatedInventory[2].Available = 3 // Premium Villa: unchanged

	updatedInvJSON, _ := json.Marshal(updatedInventory)
	db.Model(&event).Update("rooms_inventory", datatypes.JSON(updatedInvJSON))

	// 10. Create Banquet Hall
	banquet := models.BanquetHall{
		HotelID:     hotel.ID,
		Name:        "Grand Ballroom",
		Capacity:    300,
		PricePerDay: 50000.00,
	}
	db.Create(&banquet)

	// 10. Create Catering Menus
	menus := []models.CateringMenu{
		{
			HotelID:       hotel.ID,
			Name:          "Gold Veg Package",
			Type:          "veg",
			PricePerPlate: 800.00,
		},
		{
			HotelID:       hotel.ID,
			Name:          "Platinum Non-Veg Package",
			Type:          "non-veg",
			PricePerPlate: 1200.00,
		},
	}

	for _, menu := range menus {
		db.Create(&menu)
	}

	log.Println("🎉 Unified Database Reset and Seeding Completed Successfully!")
}
