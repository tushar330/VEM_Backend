package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func main() {
	// Load Env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	// Connect to DB (Custom Init to silence logger)
	// store.InitDB() uses default logger. We need to override or just re-open?
	// Re-opening might not be good if store.DB is used elsewhere, but here it's main.
	// Actually easier to just modify store.InitDB? No, don't touch prod code.
	// Let's just re-configure the logger on the existing db instance if possible.
	store.InitDB()
	db := store.DB
	db.Logger = logger.Default.LogMode(logger.Silent)

	log.Println("🌱 Starting Seed Process...")

	// 1. Create Hotel & Room Offers (Dependencies)
	hotelID := "SEED_HOTEL_001"

	// Ensure Country & City exist to prevent FK errors
	db.Exec("INSERT INTO countries (code, name, phone_code) VALUES ('SC', 'Seed Country', '999') ON CONFLICT DO NOTHING")
	db.Exec("INSERT INTO cities (id, country_code, name) VALUES ('SEED_CITY', 'SC', 'Seed City') ON CONFLICT DO NOTHING")

	hotel := models.Hotel{
		ID:         hotelID,
		CityID:     "SEED_CITY",
		Name:       "Seed Grand Hotel",
		StarRating: 5,
		Address:    "123 Seed Lane",
		Occupancy:  1000,
	}

	// Upsert Hotel
	db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&hotel)

	// Create Room Offers
	roomNames := []string{"Deluxe Room", "Suite", "Villa"}
	var roomOffers []models.RoomOffer
	for _, name := range roomNames {
		offer := models.RoomOffer{
			HotelID:     hotelID,
			Name:        name,
			BookingCode: "BK_" + name,
			TotalFare:   200.0,
			Count:       50,
		}
		// Upsert Room Offer
		if err := db.Where("hotel_id = ? AND name = ?", hotelID, name).First(&offer).Error; err != nil {
			offer.ID = uuid.New().String()
			db.Create(&offer)
		}
		roomOffers = append(roomOffers, offer)
	}

	// Create Inventory JSON for events
	inventoryItems := []map[string]interface{}{}
	for _, room := range roomOffers {
		inventoryItems = append(inventoryItems, map[string]interface{}{
			"room_offer_id":  room.ID,
			"room_name":      room.Name,
			"total":          10,
			"available":      10,
			"max_capacity":   3,
			"price_per_room": room.TotalFare,
		})
	}
	inventoryJSON, _ := json.Marshal(inventoryItems)

	// 2. Create Agent
	agentEmail := "agent@test.com"
	agentPass := "Test@123"
	hashedAgentPass, _ := bcrypt.GenerateFromPassword([]byte(agentPass), bcrypt.DefaultCost)
	agent := models.User{
		Email:        agentEmail,
		PasswordHash: string(hashedAgentPass),
		Role:         "agent",
		Name:         "Seed Agent",
		Phone:        "1234567890",
	}

	// Upsert Agent
	var existingAgent models.User
	if err := db.Where("email = ?", agentEmail).First(&existingAgent).Error; err != nil {
		agent.ID = uuid.New()
		if err := db.Create(&agent).Error; err != nil {
			log.Fatalf("Failed to create agent: %v", err)
		}
	} else {
		agent = existingAgent
		db.Model(&agent).Update("password_hash", string(hashedAgentPass))
	}

	// Ensure Agent Profile
	db.Exec("INSERT INTO agent_profiles (user_id, agency_name, agency_code) VALUES (?, 'Seed Agency', 'SA001') ON CONFLICT (user_id) DO NOTHING", agent.ID)

	// 3. Create Head Guest
	hgEmail := "headguest@test.com"
	hgPass := "Test@123"
	hashedHgPass, _ := bcrypt.GenerateFromPassword([]byte(hgPass), bcrypt.DefaultCost)
	headGuest := models.User{
		Email:        hgEmail,
		PasswordHash: string(hashedHgPass),
		Role:         "head_guest",
		Name:         "Seed Head Guest",
		Phone:        "0987654321",
	}

	// Upsert Head Guest
	var existingHG models.User
	if err := db.Where("email = ?", hgEmail).First(&existingHG).Error; err != nil {
		headGuest.ID = uuid.New()
		if err := db.Create(&headGuest).Error; err != nil {
			log.Fatalf("Failed to create head guest: %v", err)
		}
	} else {
		headGuest = existingHG
		db.Model(&headGuest).Updates(map[string]interface{}{"password_hash": string(hashedHgPass), "role": "head_guest"})
	}

	// 4. Create Events
	eventsData := []struct {
		Title  string
		Status string
		Guests int
		Alloc  bool
	}{
		{"Delhi Wedding 2026", "active", 5, false},
		{"Mumbai Corporate Event", "active", 8, true},
		{"Jaipur Destination Wedding", "finalized", 6, true},
	}

	var eventIDs []uuid.UUID

	for _, ed := range eventsData {
		eventID := uuid.New()
		event := models.Event{
			ID:             eventID,
			AgentID:        agent.ID,
			HeadGuestID:    headGuest.ID,
			HotelID:        hotelID,
			Name:           ed.Title,
			Location:       "Seed Location",
			RoomsInventory: datatypes.JSON(inventoryJSON),
			Status:         ed.Status,
			StartDate:      time.Now().AddDate(0, 1, 0),
			EndDate:        time.Now().AddDate(0, 1, 5),
		}

		var existingEvent models.Event
		if err := db.Where("agent_id = ? AND name = ?", agent.ID, ed.Title).First(&existingEvent).Error; err == nil {
			event.ID = existingEvent.ID
			db.Model(&existingEvent).Updates(map[string]interface{}{
				"status":          ed.Status,
				"head_guest_id":   headGuest.ID,
				"rooms_inventory": datatypes.JSON(inventoryJSON),
			})
		} else {
			if err := db.Create(&event).Error; err != nil {
				log.Fatalf("Failed to create event %s: %v", ed.Title, err)
			}
		}
		eventIDs = append(eventIDs, event.ID)

		// Check Guests
		var guestCount int64
		db.Model(&models.Guest{}).Where("event_id = ?", event.ID).Count(&guestCount)

		if guestCount == 0 {
			familyID := uuid.New()
			for i := 1; i <= ed.Guests; i++ {
				if i%3 == 1 {
					familyID = uuid.New()
				}
				guest := models.Guest{
					ID:       uuid.New(),
					EventID:  event.ID,
					FamilyID: familyID,
					Name:     fmt.Sprintf("Guest %d - %s", i, ed.Title),
					Type:     "adult",
					Email:    fmt.Sprintf("guest%d_%s@test.com", i, event.ID.String()[:4]),
				}
				db.Create(&guest)

				if ed.Alloc && i <= 3 {
					room := roomOffers[i%len(roomOffers)]
					allocation := models.GuestAllocation{
						ID:           uuid.New(),
						EventID:      event.ID,
						GuestID:      guest.ID,
						RoomOfferID:  &room.ID,
						Status:       "allocated",
						AssignedMode: "agent_manual",
						LockedPrice:  room.TotalFare,
					}
					db.Create(&allocation)
				}
			}
		} else {
			if ed.Alloc {
				var allocCount int64
				db.Model(&models.GuestAllocation{}).Where("event_id = ?", event.ID).Count(&allocCount)
				if allocCount == 0 {
					var guests []models.Guest
					db.Where("event_id = ?", event.ID).Find(&guests)
					for i, g := range guests {
						if i < 3 {
							room := roomOffers[i%len(roomOffers)]
							allocation := models.GuestAllocation{
								ID:           uuid.New(),
								EventID:      event.ID,
								GuestID:      g.ID,
								RoomOfferID:  &room.ID,
								Status:       "allocated",
								AssignedMode: "agent_manual",
								LockedPrice:  room.TotalFare,
							}
							db.Create(&allocation)
						}
					}
				}
			}
		}
	}

	// Output Credentials
	fmt.Println("\n=== SEED COMPLETE ===")
	fmt.Println("\nAGENT LOGIN:")
	fmt.Printf("Email: %s\n", agentEmail)
	fmt.Printf("Password: %s\n", agentPass)

	fmt.Println("\nHEAD GUEST LOGIN:")
	fmt.Printf("Email: %s\n", hgEmail)
	fmt.Printf("Password: %s\n", hgPass)

	fmt.Println("\nEVENT IDS:")
	for i, ed := range eventsData {
		fmt.Printf("%s → %s\n", ed.Title, eventIDs[i])
	}
	fmt.Println("\n=====================")
}
