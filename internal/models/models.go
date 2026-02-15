package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// User represents a registered user (Head Guest or Agent)
type User struct {
	ID           uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name         string       `json:"name"`
	Email        string       `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string       `gorm:"not null" json:"-"` // For custom auth
	Phone        string       `json:"phone"`
	Role         string       `gorm:"default:'head_guest'" json:"role"` // 'agent' or 'head_guest'
	AgentProfile AgentProfile `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"agent_profile"`
}

type AgentProfile struct {
	UserID        uuid.UUID `gorm:"primaryKey;type:uuid" json:"user_id"`
	AgencyName    string    `json:"agency_name"`
	AgencyCode    string    `json:"agency_code"`
	Location      string    `json:"location"`
	BusinessPhone string    `json:"business_phone"`
}

type Guest struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name          string    `gorm:"not null" json:"name"`
	Age           int       `json:"age"`
	Type          string    `gorm:"default:'adult'" json:"type"` // 'adult' or 'child'
	Phone         string    `json:"phone"`
	Email         string    `json:"email"`
	EventID       uuid.UUID `gorm:"type:uuid;index;not null" json:"event_id"`
	FamilyID      uuid.UUID `gorm:"type:uuid;index;not null" json:"family_id"` // REQUIRED for family-based allocation
	ArrivalDate   time.Time `json:"arrival_date"`
	DepartureDate time.Time `json:"departure_date"`
}

func (g *Guest) BeforeSave(tx *gorm.DB) (err error) {
	if g.Type == "" {
		if g.Age >= 12 {
			g.Type = "adult"
		} else {
			g.Type = "child"
		}
	}
	return
}

type Event struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AgentID        uuid.UUID      `gorm:"type:uuid;index" json:"agent_id"`
	HeadGuestID    uuid.UUID      `gorm:"type:uuid;index" json:"head_guest_id"`
	HotelID        string         `gorm:"index" json:"hotel_id"`
	Name           string         `gorm:"not null" json:"name"`
	Location       string         `json:"location"`
	RoomsInventory datatypes.JSON `gorm:"type:jsonb" json:"rooms_inventory"`
	Status         string         `gorm:"default:'draft'" json:"status"`
	StartDate      time.Time      `json:"start_date"`
	EndDate        time.Time      `json:"end_date"`
}

// 1. Country Table (Global)
type Country struct {
	Code      string `gorm:"primaryKey;size:2" json:"code"` // ISO Code 'US', 'IN'
	Name      string `gorm:"not null" json:"name"`
	PhoneCode string `gorm:"size:10" json:"phone_code"`
	// Relations
	Cities []City `gorm:"foreignKey:CountryCode" json:"cities"`
}

// 2. City Table (Global)
type City struct {
	ID          string `gorm:"primaryKey" json:"id"` // Unique Code
	CountryCode string `gorm:"size:2;index;not null" json:"country_code"`
	Name        string `gorm:"index;not null" json:"name"`
	IsPopular   bool   `gorm:"default:false" json:"is_popular"`
	// Relations
	Hotels []Hotel `gorm:"foreignKey:CityID" json:"hotels"`
}

// 3. Hotel Static Data (Global)
type Hotel struct {
	ID         string         `gorm:"primaryKey;column:hotel_code" json:"id"` // TBO Code e.g. "1279415"
	CityID     string         `gorm:"index;not null" json:"city_id"`
	Name       string         `gorm:"not null" json:"name"`
	StarRating int            `gorm:"default:0" json:"star_rating"`
	Address    string         `gorm:"type:text" json:"address"`
	Facilities datatypes.JSON `gorm:"type:jsonb" json:"facilities"` // Stores ["Wifi", "Pool"]
	ImageUrls  datatypes.JSON `gorm:"type:jsonb" json:"image_urls"` // Stores ["url1.jpg", "url2.jpg"]
	Occupancy  int            `gorm:"default:500" json:"occupancy"`

	// Relations
	Rooms    []RoomOffer    `gorm:"foreignKey:HotelID" json:"rooms"`
	Banquets []BanquetHall  `gorm:"foreignKey:HotelID" json:"banquets"`
	Menus    []CateringMenu `gorm:"foreignKey:HotelID" json:"menus"`
}

// 4. Room Offers (Global Static)
type RoomOffer struct {
	ID      string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	HotelID string `gorm:"index;not null" json:"hotel_id"` // Links to Hotel.ID
	Name    string `gorm:"not null" json:"name"`           // "Ocean King"

	BookingCode    string         `gorm:"not null" json:"booking_code"` // API Booking Key
	MaxCapacity    int            `gorm:"not null;default:2" json:"max_capacity"`
	TotalFare      float64        `gorm:"type:decimal(10,2);not null" json:"total_fare"`
	Currency       string         `gorm:"size:3;default:'USD'" json:"currency"`
	IsRefundable   bool           `gorm:"default:false" json:"is_refundable"`
	CancelPolicies datatypes.JSON `gorm:"type:jsonb" json:"cancel_policies"` // Stores the complex policy array
	Count          int            `gorm:"default:100" json:"count"`
}

// 5. Banquet Halls
type BanquetHall struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	HotelID     string         `gorm:"index;not null" json:"hotel_id"`
	Name        string         `gorm:"not null" json:"name"`
	Capacity    int            `gorm:"not null" json:"capacity"`
	PricePerDay float64        `gorm:"type:decimal(10,2)" json:"price_per_day"`
	ImageUrls   datatypes.JSON `gorm:"type:jsonb" json:"image_urls"`
}

// 6. Catering Menus
type CateringMenu struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	HotelID       string         `gorm:"index;not null" json:"hotel_id"`
	Name          string         `gorm:"not null" json:"name"`      // "Gold Package"
	Type          string         `gorm:"default:'veg'" json:"type"` // 'veg', 'non-veg'
	PricePerPlate float64        `gorm:"type:decimal(10,2)" json:"price_per_plate"`
	ImageUrls     datatypes.JSON `gorm:"type:jsonb" json:"image_urls"`
}

// 7. Guest Allocation (The "Join" Table)
type GuestAllocation struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// Links to your EXISTING tables
	EventID uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_event_guest" json:"event_id"`
	Event   Event     `gorm:"foreignKey:EventID" json:"-"`

	GuestID uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:idx_event_guest" json:"guest_id"`
	Guest   Guest     `gorm:"foreignKey:GuestID" json:"guest"`

	// Links to the NEW table
	RoomOfferID *string   `gorm:"type:uuid;index" json:"room_offer_id"`
	RoomOffer   RoomOffer `gorm:"foreignKey:RoomOfferID" json:"room_offer"`

	// The Logic Columns
	LockedPrice  float64 `gorm:"type:decimal(10,2)" json:"locked_price"`      // Audit - price locked at allocation
	Status       string  `gorm:"default:'allocated'" json:"status"`           // 'allocated', 'checked_in', 'checked_out', 'cancelled'
	AssignedMode string  `gorm:"default:'agent_manual'" json:"assigned_mode"` // 'agent_manual', 'head_guest_manual'
}
