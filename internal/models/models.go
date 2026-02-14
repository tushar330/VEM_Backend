package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// User represents a registered user (Head Guest or Agent)
type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name         string
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"` // For custom auth
	Phone        string
	Role         string       `gorm:"default:'head_guest'"` // 'agent' or 'head_guest'
	AgentProfile AgentProfile `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type AgentProfile struct {
	UserID        uuid.UUID `gorm:"primaryKey;type:uuid"`
	AgencyName    string
	AgencyCode    string
	Location      string
	BusinessPhone string
}

type Guest struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name          string    `gorm:"not null"`
	Age           int
	Type          string `gorm:"default:'adult'"` // 'adult' or 'child'
	Phone         string
	Email         string
	EventID       uuid.UUID `gorm:"type:uuid;index;not null"`
	ArrivalDate   time.Time
	DepartureDate time.Time
}

func (g *Guest) BeforeSave(tx *gorm.DB) (err error) {
	if g.Age >= 12 {
		g.Type = "Adult"
	} else if g.Age < 12 {
		g.Type = "Child"
	}
	return
}

type Event struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AgentID        uuid.UUID      `gorm:"type:uuid;index" json:"agentId"`
	HeadGuestID    uuid.UUID      `gorm:"type:uuid;index" json:"headGuestId"`
	HotelID        string         `gorm:"index" json:"hotelId"`
	Name           string         `gorm:"not null" json:"name"`
	Location       string         `json:"location"`
	RoomsInventory datatypes.JSON `gorm:"type:jsonb" json:"roomsInventory"`
	Status         string         `gorm:"default:'draft'" json:"status"`
	StartDate      time.Time      `json:"startDate"`
	EndDate        time.Time      `json:"endDate"`
}


// 1. Country Table (Global)
type Country struct {
	Code      string `gorm:"primaryKey;size:2"` // ISO Code 'US', 'IN'
	Name      string `gorm:"not null"`
	PhoneCode string `gorm:"size:10"`
	// Relations
	Cities []City `gorm:"foreignKey:CountryCode"`
}

// 2. City Table (Global)
type City struct {
	ID          string `gorm:"primaryKey"` // Unique Code
	CountryCode string `gorm:"size:2;index;not null"`
	Name        string `gorm:"index;not null"`
	IsPopular   bool   `gorm:"default:false"`
	// Relations
	Hotels []Hotel `gorm:"foreignKey:CityID"`
}

// 3. Hotel Static Data (Global)
type Hotel struct {
	ID         string         `gorm:"primaryKey;column:hotel_code"` // TBO Code e.g. "1279415"
	CityID     string         `gorm:"index;not null"`
	Name       string         `gorm:"not null"`
	StarRating int            `gorm:"default:0"`
	Address    string         `gorm:"type:text"`
	Facilities datatypes.JSON `gorm:"type:jsonb"` // Stores ["Wifi", "Pool"]
	ImageUrls  datatypes.JSON `gorm:"type:jsonb"` // Stores ["url1.jpg", "url2.jpg"]
	Occupancy  int            `gorm:"default:500"`

	// Relations
	Rooms    []RoomOffer    `gorm:"foreignKey:HotelID"`
	Banquets []BanquetHall  `gorm:"foreignKey:HotelID"`
	Menus    []CateringMenu `gorm:"foreignKey:HotelID"`
}

// 4. Room Offers (Global Static)
type RoomOffer struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	HotelID        string         `gorm:"index;not null"` // Links to Hotel.ID
	Name           string         `gorm:"not null"`       // "Ocean King"
	
	BookingCode    string         `gorm:"not null"`       // API Booking Key
	TotalFare      float64        `gorm:"type:decimal(10,2);not null"`
	Currency       string         `gorm:"size:3;default:'USD'"`
	IsRefundable   bool           `gorm:"default:false"`
	CancelPolicies datatypes.JSON `gorm:"type:jsonb"` // Stores the complex policy array
	Count          int            `gorm:"default:100"`
	MaxCapacity    int            `gorm:"default:2"`
}

// 5. Banquet Halls
type BanquetHall struct {
	ID          uint    `gorm:"primaryKey"`
	HotelID     string  `gorm:"index;not null"`
	Name        string  `gorm:"not null"`
	Capacity    int     `gorm:"not null"`
	PricePerDay float64 `gorm:"type:decimal(10,2)"`
}

// 6. Catering Menus
type CateringMenu struct {
	ID            uint    `gorm:"primaryKey"`
	HotelID       string  `gorm:"index;not null"`
	Name          string  `gorm:"not null"`      // "Gold Package"
	Type          string  `gorm:"default:'veg'"` // 'veg', 'non-veg'
	PricePerPlate float64 `gorm:"type:decimal(10,2)"`
}

// 7. Guest Allocation (The "Join" Table)
type GuestAllocation struct {
	ID uint `gorm:"primaryKey"`

	// Links to your EXISTING tables
	EventID uuid.UUID `gorm:"type:uuid;index;not null"`
	Event   Event     `gorm:"foreignKey:EventID"`

	GuestID uuid.UUID `gorm:"type:uuid;index;not null"`
	Guest   Guest     `gorm:"foreignKey:GuestID"`

	// Links to the NEW table
	RoomOfferID *string   `gorm:"type:uuid;index"`
	RoomOffer   RoomOffer `gorm:"foreignKey:RoomOfferID"`

	// The Logic Columns
	VirtualRoomID int     `gorm:"index"`              // Roommate logic
	LockedPrice   float64 `gorm:"type:decimal(10,2)"` // Audit
	Status        string  `gorm:"default:'pending'"`  // 'confirmed', 'checked_in'
	AssignedMode  string  `gorm:"default:'manual'"`   // 'auto', 'manual'
}
