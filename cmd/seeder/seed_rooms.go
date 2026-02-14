package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
)

// SeedRooms generates mock room data for hotels and populates the database
func SeedRooms(limit int) {
	log.Printf("🛏️  Generating MOCK room data (Limit: %d hotels)...", limit)

	// Step 0: Clear existing data and Update Schema
	log.Println("🧹 Clearing existing room offers and updating schema...")

	// Drop the table to ensure schema changes are applied correctly.
	if err := store.DB.Exec("DROP TABLE IF EXISTS room_offers CASCADE").Error; err != nil {
		log.Printf("⚠️  Error dropping table: %v", err)
	}

	// Re-create table with new schema
	if err := store.DB.AutoMigrate(&models.RoomOffer{}); err != nil {
		log.Printf("❌ Error migrating table room_offers: %v", err)
		return
	}

	// Also migrate GuestAllocation because it references RoomOffer
	store.DB.Exec("DROP TABLE IF EXISTS guest_allocations CASCADE")
	if err := store.DB.AutoMigrate(&models.GuestAllocation{}); err != nil {
		log.Printf("⚠️ Error migrating guest_allocations: %v", err)
	}

	log.Println("✅ Room offers table recreated and schema updated.")

	// Get hotels from database
	var hotels []models.Hotel
	// ID is mapped to hotel_code in the model
	query := store.DB.Select("hotel_code, name").Order("hotel_code")
	if limit > 0 {
		query = query.Limit(limit)
	}
	result := query.Find(&hotels)
	if result.Error != nil {
		log.Printf("❌ Error fetching hotels: %v", result.Error)
		return
	}

	log.Printf("📋 Found %d hotels to process", len(hotels))

	var batchRoomOffers []models.RoomOffer
	totalInserted := int64(0)

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	for i, hotel := range hotels {
		if (i+1)%50 == 0 {
			log.Printf("🏨 Processing hotel %d/%d...", i+1, len(hotels))
		}

		// Generate 3 room types * 2 capacities = 6 rooms per hotel
		mockRooms := generateMockRooms(hotel.ID)
		batchRoomOffers = append(batchRoomOffers, mockRooms...)

		// Check if batch is ready to save
		if len(batchRoomOffers) >= 500 { // Save every ~500 records
			count := saveRoomBatch(batchRoomOffers)
			totalInserted += count
			batchRoomOffers = nil // Clear slice
		}
	}

	// Save remaining rooms
	if len(batchRoomOffers) > 0 {
		count := saveRoomBatch(batchRoomOffers)
		totalInserted += count
	}

	log.Printf("🎉 Finished! Processed %d hotels. Total rooms inserted: %d", len(hotels), totalInserted)
}

func saveRoomBatch(rooms []models.RoomOffer) int64 {
	if len(rooms) == 0 {
		return 0
	}
	result := store.DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rooms, 100)
	if result.Error != nil {
		log.Printf("❌ Error batch inserting rooms: %v", result.Error)
		return 0
	}
	return result.RowsAffected
}

// generateMockRooms creates 3 room types with 2 capacities each
func generateMockRooms(hotelID string) []models.RoomOffer {
	var offers []models.RoomOffer

	roomTypes := []struct {
		Name       string
		BasePrice  float64
		Refundable bool
	}{
		{"Standard Room", 100.0, false},
		{"Deluxe Room", 200.0, true},
		{"Suite", 400.0, true},
	}

	capacities := []int{2, 4}

	for _, rt := range roomTypes {
		for _, cap := range capacities {
			// Randomize price slightly (+- 20%)
			variation := (rand.Float64() * 0.4) + 0.8                   // 0.8 to 1.2
			finalPrice := rt.BasePrice * float64(cap) * 0.7 * variation // Cheaper per person for larger rooms

			// Random count 100-150
			count := rand.Intn(51) + 100

			// Generate Cancel Policy
			policy := []map[string]interface{}{}
			if rt.Refundable {
				policy = append(policy, map[string]interface{}{
					"ChargeType":         "Percentage",
					"CancellationCharge": 10.0,
					"FromDate":           time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
				})
			} else {
				policy = append(policy, map[string]interface{}{
					"ChargeType":         "Percentage",
					"CancellationCharge": 100.0,
					"FromDate":           time.Now().Format("2006-01-02"),
				})
			}
			policyJSON, _ := json.Marshal(policy)

			offer := models.RoomOffer{
				ID:             uuid.New().String(),
				HotelID:        hotelID,
				Name:           fmt.Sprintf("%s (%d Pax)", rt.Name, cap),
				BookingCode:    fmt.Sprintf("MOCK-%s-%d-%s", hotelID, cap, uuid.New().String()[:8]),
				TotalFare:      finalPrice,
				Currency:       "USD",
				IsRefundable:   rt.Refundable,
				CancelPolicies: datatypes.JSON(policyJSON),
				Count:          count,
				MaxCapacity:    cap,
			}
			offers = append(offers, offer)
		}
	}

	return offers
}
