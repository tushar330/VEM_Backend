package main

import (
	"flag"
	"fmt"
	"log"

	// "github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	_ = flag.String("type", "", "Type of seeding to run: countries, cities, hotels, rooms")
	flag.Parse()

	store.InitDB()

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
		if err := store.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)).Error; err != nil {
			log.Fatalf("❌ Failed to drop table %s: %v", table, err)
		}
	}
	log.Println("✅ Database Cleared.")

	// 2. Automigration
	log.Println("🛠️  Running Automigrate...")
	err := store.DB.AutoMigrate(
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

	// 3. Constrained Seeding
	log.Println("🌱 Starting Constrained Seeding...")

	targetCountries := []string{"US", "SG", "AE", "TH", "IN"}

	// Step 1: Countries
	SeedCountries(targetCountries)

	// Step 2: Cities (IN: 200, others: 100)
	SeedCities(targetCountries)

	// Step 3: Hotels (50 per city)
	SeedHotels(50)

	// Step 4: Rooms
	SeedRooms(0) // Process all seeded hotels

	log.Println("🎉 Unified Database Reset and Seeding Completed Successfully!")
}
