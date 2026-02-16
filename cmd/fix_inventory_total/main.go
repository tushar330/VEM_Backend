package main

import (
	"encoding/json"
	"log"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/joho/godotenv"
	"gorm.io/datatypes"
)

type RoomsInventoryItem struct {
	RoomOfferID  string `json:"room_offer_id"`
	RoomName     string `json:"room_name"`
	Total        int    `json:"total"`
	Available    int    `json:"available"`
	MaxCapacity  int    `json:"max_capacity"`
	PricePerRoom int    `json:"price_per_room"`
}

func main() {
	// 1. Load Environment Variables
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️  Warning: .env file not found")
	}

	// 2. Initialize Database
	store.InitDB()
	db := store.DB

	log.Println("🚀 Starting Inventory Fix Migration...")

	// 3. Fetch All Events
	var events []models.Event
	if err := db.Find(&events).Error; err != nil {
		log.Fatalf("❌ Failed to fetch events: %v", err)
	}

	log.Printf("Found %d events to process.", len(events))

	for _, event := range events {
		var inventory []RoomsInventoryItem
		if len(event.RoomsInventory) == 0 {
			continue
		}

		if err := json.Unmarshal(event.RoomsInventory, &inventory); err != nil {
			log.Printf("❌ Failed to unmarshal inventory for event %s: %v", event.ID, err)
			continue
		}

		updated := false
		for i, item := range inventory {
			// Check if Total is missing (0) or suspiciously low (Total < Available)
			// Logic: Total should be at least Available
			if item.Total == 0 || item.Total < item.Available {
				// Query Allocations count
				var allocatedCount int64
				err := db.Model(&models.GuestAllocation{}).
					Where("event_id = ? AND room_offer_id = ?", event.ID, item.RoomOfferID).
					Count(&allocatedCount).Error

				if err != nil {
					log.Printf("❌ Failed to count allocations for event %s room %s: %v", event.ID, item.RoomOfferID, err)
					continue
				}

				newTotal := item.Available + int(allocatedCount)

				// Update Item
				inventory[i].Total = newTotal
				updated = true

				log.Printf("🔧 Fixed Event %s | Room %s | Available: %d | Allocated: %d | New Total: %d",
					event.ID, item.RoomName, item.Available, allocatedCount, newTotal)
			}
		}

		if updated {
			newJSON, err := json.Marshal(inventory)
			if err != nil {
				log.Printf("❌ Failed to marshal new inventory for event %s: %v", event.ID, err)
				continue
			}

			if err := db.Model(&event).Update("rooms_inventory", datatypes.JSON(newJSON)).Error; err != nil {
				log.Printf("❌ Failed to save event %s: %v", event.ID, err)
			} else {
				log.Printf("✅ Saved updates for event %s", event.ID)
			}
		} else {
			log.Printf("⏩ No changes needed for event %s", event.ID)
		}
	}

	log.Println("✅ Migration Complete.")
}
