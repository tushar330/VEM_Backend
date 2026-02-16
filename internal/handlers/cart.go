package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// --- Request DTOs ---

type AddToCartRequest struct {
	Type     string `json:"type"`     // 'room', 'banquet', 'catering', 'flight'
	RefID    string `json:"refId"`    // ID of the referenced item
	Quantity int    `json:"quantity"` // Number of units (default: 1)
	Notes    string `json:"notes"`    // Optional notes
	Status   string `json:"status"`   // Optional status (default: 'wishlist')
}

type UpdateCartItemRequest struct {
	Quantity *int    `json:"quantity,omitempty"` // Optional quantity update
	Notes    *string `json:"notes,omitempty"`    // Optional notes update
	Status   *string `json:"status,omitempty"`   // Optional status update
}

// AddToCart adds an item to the event cart/wishlist
func (m *Repository) AddToCart(c *fiber.Ctx) error {
	eventID := c.Params("id")
	userID := c.Locals("userID")
	if userID == nil {
		return utils.ErrorResponse(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	currentUserID, ok := userID.(uuid.UUID)
	if !ok {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Invalid User ID type")
	}

	// Validate event ID
	if _, err := uuid.Parse(eventID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}

	// Guard: Check Event Status (New Lifecycle)
	var event models.Event
	if err := m.DB.Where("id = ?", eventID).First(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}
	if event.Status == "finalized" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Event is finalized and locked")
	}

	// Parse request
	var req AddToCartRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// Validate required fields
	if req.Type == "" || req.RefID == "" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Missing required fields: type and refId")
	}

	// Default quantity to 1 if not provided
	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	// Default status if not provided
	if req.Status == "" {
		req.Status = "wishlist"
	}

	// Determine parent_hotel_id and fetch locked price based on type
	var parentHotelID *string
	var lockedPrice float64

	switch req.Type {
	case "room":
		var room models.RoomOffer
		log.Printf("🔍 [CART] Searching for RoomOffer ID: %s\n", req.RefID)
		if err := m.DB.Where("id = ?", req.RefID).First(&room).Error; err != nil {
			log.Printf("❌ [CART] RoomOffer NOT FOUND: %s (Error: %v)\n", req.RefID, err)
			return utils.ErrorResponse(c, fiber.StatusNotFound, "Room offer not found")
		}
		parentHotelID = &room.HotelID
		lockedPrice = room.TotalFare

	case "banquet":
		var banquet models.BanquetHall
		if err := m.DB.Where("id = ?", req.RefID).First(&banquet).Error; err != nil {
			return utils.ErrorResponse(c, fiber.StatusNotFound, "Banquet hall not found")
		}
		parentHotelID = &banquet.HotelID
		lockedPrice = banquet.PricePerDay

	case "catering":
		var catering models.CateringMenu
		if err := m.DB.Where("id = ?", req.RefID).First(&catering).Error; err != nil {
			return utils.ErrorResponse(c, fiber.StatusNotFound, "Catering menu not found")
		}
		parentHotelID = &catering.HotelID
		lockedPrice = catering.PricePerPlate

	case "hotel":
		var hotel models.Hotel
		log.Printf("🔍 [CART] Searching for Hotel Code: %s\n", req.RefID)
		if err := m.DB.Where("hotel_code = ?", req.RefID).First(&hotel).Error; err != nil {
			log.Printf("❌ [CART] Hotel NOT FOUND: %s (Error: %v)\n", req.RefID, err)
			return utils.ErrorResponse(c, fiber.StatusNotFound, "Hotel not found")
		}
		parentHotelID = &hotel.ID
		lockedPrice = 0

	case "flight":
		// Flights don't have a parent hotel
		parentHotelID = nil
		lockedPrice = 0 // Flight pricing would be handled separately

	default:
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid type. Must be: hotel, room, banquet, catering, or flight")
	}

	// Create cart item
	eventUUID, _ := uuid.Parse(eventID)
	cartItem := models.CartItem{
		EventID:       eventUUID,
		Type:          req.Type,
		RefID:         req.RefID,
		ParentHotelID: parentHotelID,
		Status:        req.Status,
		Quantity:      req.Quantity,
		LockedPrice:   lockedPrice,
		Notes:         req.Notes,
		AddedBy:       currentUserID,
	}

	if err := m.DB.Create(&cartItem).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to add item to cart")
	}

	// Invalidate Cache
	utils.Invalidate(context.Background(), fmt.Sprintf("cart:event:%s", eventID))

	return utils.SuccessResponse(c, fiber.StatusCreated, fiber.Map{
		"message": "Item added to cart successfully",
		"item":    cartItem,
	})
}

// GetEventCart retrieves all cart items for an event with hierarchical grouping
func (r *Repository) GetEventCart(c *fiber.Ctx) error {
	eventID := c.Params("id")
	status := c.Query("status") // Optional filter by status

	// Validate event ID
	if _, err := uuid.Parse(eventID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}

	var response models.CartResponse
	cacheKey := fmt.Sprintf("cart:event:%s", eventID)
	if status != "" {
		cacheKey += ":status:" + status
	}
	ctx := context.Background()

	// 1. Try to get from Redis
	if store.RDB != nil {
		cachedData, err := store.RDB.Get(ctx, cacheKey).Result()
		if err == nil {
			if err := json.Unmarshal([]byte(cachedData), &response); err == nil {
				log.Printf("⚡ [REDIS] CACHE HIT: %s\n", cacheKey)
				return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
					"message": "Cart fetched successfully (Cached)",
					"cart":    response,
				})
			}
		} else {
			log.Printf("🔍 [REDIS] CACHE MISS: %s (Reason: %v)\n", cacheKey, err)
		}
	}

	// Fetch cart items
	var cartItems []models.CartItem
	query := r.DB.Where("event_id = ?", eventID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("created_at DESC").Find(&cartItems).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to fetch cart items")
	}

	// Build hierarchical response
	var err error
	response, err = r.buildHierarchicalCartResponse(eventID, cartItems)
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to build cart response: "+err.Error())
	}

	// 2. Store in Redis
	if store.RDB != nil {
		if data, err := json.Marshal(response); err == nil {
			store.RDB.Set(ctx, cacheKey, data, 1*time.Hour)
			log.Printf("💾 [REDIS] CACHE SET: %s\n", cacheKey)
		}
	}

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Cart fetched successfully",
		"cart":    response,
	})
}

// UpdateCartItem updates a cart item's quantity, notes, or status
func (m *Repository) UpdateCartItem(c *fiber.Ctx) error {
	eventID := c.Params("id")
	cartItemID := c.Params("cartItemId")

	// Validate IDs
	if _, err := uuid.Parse(eventID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}
	if _, err := uuid.Parse(cartItemID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Cart Item ID")
	}

	// Guard: Check Event Status
	var event models.Event
	if err := m.DB.Where("id = ?", eventID).First(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}
	if event.Status == "finalized" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Event is finalized and locked")
	}

	// Parse request
	var req UpdateCartItemRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid request body")
	}

	// Find cart item
	var cartItem models.CartItem
	if err := m.DB.Where("id = ? AND event_id = ?", cartItemID, eventID).First(&cartItem).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Cart item not found")
	}

	// Build updates map
	updates := map[string]interface{}{}
	if req.Quantity != nil && *req.Quantity > 0 {
		updates["quantity"] = *req.Quantity
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// Apply updates
	if len(updates) > 0 {
		if err := m.DB.Model(&cartItem).Updates(updates).Error; err != nil {
			return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to update cart item")
		}

		// Invalidate Cache
		utils.Invalidate(context.Background(), fmt.Sprintf("cart:event:%s", eventID))
	}

	// Fetch updated item
	m.DB.Where("id = ?", cartItemID).First(&cartItem)

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Cart item updated successfully",
		"item":    cartItem,
	})
}

// RemoveFromCart removes a cart item
func (m *Repository) RemoveFromCart(c *fiber.Ctx) error {
	eventID := c.Params("id")
	cartItemID := c.Params("cartItemId")

	// Validate IDs
	if _, err := uuid.Parse(eventID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}
	if _, err := uuid.Parse(cartItemID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Cart Item ID")
	}

	// Guard: Check Event Status
	var event models.Event
	if err := m.DB.Where("id = ?", eventID).First(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}
	if event.Status == "finalized" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Event is finalized and locked")
	}

	// Delete cart item
	result := m.DB.Where("id = ? AND event_id = ?", cartItemID, eventID).Delete(&models.CartItem{})
	if result.Error != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to remove cart item")
	}
	if result.RowsAffected == 0 {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Cart item not found")
	}

	// Invalidate Cache
	utils.Invalidate(context.Background(), fmt.Sprintf("cart:event:%s", eventID))

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message": "Cart item removed successfully",
	})
}

// UpdateCartStatus approves all wishlist items (converts wishlist to approved cart)
func (m *Repository) UpdateCartStatus(c *fiber.Ctx) error {
	eventID := c.Params("id")

	// Validate event ID
	if _, err := uuid.Parse(eventID); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Invalid Event ID")
	}

	// Guard: Check Event Status
	var event models.Event
	if err := m.DB.Where("id = ?", eventID).First(&event).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "Event not found")
	}
	if event.Status == "finalized" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "Event is finalized and locked")
	}

	// Update all wishlist items to approved
	result := m.DB.Model(&models.CartItem{}).
		Where("event_id = ? AND status = ?", eventID, "wishlist").
		Update("status", "approved")

	if result.Error != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "Failed to approve cart items")
	}

	// Invalidate Cache
	utils.Invalidate(context.Background(), fmt.Sprintf("cart:event:%s", eventID))

	return utils.SuccessResponse(c, fiber.StatusOK, fiber.Map{
		"message":       "Cart approved successfully",
		"items_updated": result.RowsAffected,
	})
}

// --- Helper Functions ---

// buildHierarchicalCartResponse constructs the hierarchical response grouped by hotel
func (m *Repository) buildHierarchicalCartResponse(eventID string, cartItems []models.CartItem) (models.CartResponse, error) {
	eventUUID, _ := uuid.Parse(eventID)
	response := models.CartResponse{
		EventID: eventUUID,
		Hotels:  []models.HotelCartGroup{},
		Flights: []models.CartItemDetail{},
	}

	// Group items by hotel
	hotelGroups := make(map[string]*models.HotelCartGroup)

	// Collect all IDs for batch fetching
	var roomIDs, banquetIDs, cateringIDs []string
	var hotelIDs []string

	for _, item := range cartItems {
		if item.Type == "flight" {
			// Add flights directly to response
			response.Flights = append(response.Flights, models.CartItemDetail{
				ID:          item.ID,
				Type:        item.Type,
				Status:      item.Status,
				Quantity:    item.Quantity,
				LockedPrice: item.LockedPrice,
				Notes:       item.Notes,
				CreatedAt:   item.CreatedAt,
			})
		} else {
			// Group by hotel
			hotelID := ""
			if item.ParentHotelID != nil {
				hotelID = *item.ParentHotelID
			}

			if _, exists := hotelGroups[hotelID]; !exists {
				hotelGroups[hotelID] = &models.HotelCartGroup{
					Rooms:    []models.CartItemDetail{},
					Banquets: []models.CartItemDetail{},
					Catering: []models.CartItemDetail{},
				}
				if hotelID != "" {
					hotelIDs = append(hotelIDs, hotelID)
				}
			}

			// Collect IDs for batch fetch
			switch item.Type {
			case "room":
				roomIDs = append(roomIDs, item.RefID)
			case "banquet":
				banquetIDs = append(banquetIDs, item.RefID)
			case "catering":
				cateringIDs = append(cateringIDs, item.RefID)
			}
		}
	}

	// Batch fetch all details
	rooms := m.fetchRoomDetails(roomIDs)
	banquets := m.fetchBanquetDetails(banquetIDs)
	caterings := m.fetchCateringDetails(cateringIDs)
	hotels := m.fetchHotelDetails(hotelIDs)

	// Map items to groups with details
	for _, item := range cartItems {
		if item.Type == "flight" {
			continue // Already handled
		}

		hotelID := ""
		if item.ParentHotelID != nil {
			hotelID = *item.ParentHotelID
		}

		cartDetail := models.CartItemDetail{
			ID:          item.ID,
			Type:        item.Type,
			Status:      item.Status,
			Quantity:    item.Quantity,
			LockedPrice: item.LockedPrice,
			Notes:       item.Notes,
			CreatedAt:   item.CreatedAt,
		}

		switch item.Type {
		case "room":
			if roomDetail, ok := rooms[item.RefID]; ok {
				cartDetail.RoomDetails = roomDetail
			}
			hotelGroups[hotelID].Rooms = append(hotelGroups[hotelID].Rooms, cartDetail)

		case "banquet":
			if banquetDetail, ok := banquets[item.RefID]; ok {
				cartDetail.BanquetDetails = banquetDetail
			}
			hotelGroups[hotelID].Banquets = append(hotelGroups[hotelID].Banquets, cartDetail)

		case "catering":
			if cateringDetail, ok := caterings[item.RefID]; ok {
				cartDetail.CateringDetails = cateringDetail
			}
			hotelGroups[hotelID].Catering = append(hotelGroups[hotelID].Catering, cartDetail)

		case "hotel":
			if hotelDetail, ok := hotels[item.RefID]; ok {
				cartDetail.HotelDetails = hotelDetail
			}
			hotelGroups[hotelID].HotelWishlistItem = &cartDetail
		}
	}

	// Build final hotel groups with hotel details
	for hotelID, group := range hotelGroups {
		if hotelDetail, ok := hotels[hotelID]; ok {
			group.HotelDetails = hotelDetail
		}
		response.Hotels = append(response.Hotels, *group)
	}

	return response, nil
}

// fetchHotelDetails fetches hotel details for given hotel IDs
func (m *Repository) fetchHotelDetails(hotelIDs []string) map[string]models.Hotel {
	result := make(map[string]models.Hotel)
	if len(hotelIDs) == 0 {
		return result
	}

	var hotels []models.Hotel
	m.DB.Where("hotel_code IN ?", hotelIDs).Find(&hotels)

	for _, hotel := range hotels {
		result[hotel.ID] = hotel
	}
	return result
}

// fetchRoomDetails fetches room offer details for given room IDs
func (m *Repository) fetchRoomDetails(roomIDs []string) map[string]models.RoomOffer {
	result := make(map[string]models.RoomOffer)
	if len(roomIDs) == 0 {
		return result
	}

	var rooms []models.RoomOffer
	m.DB.Where("id IN ?", roomIDs).Find(&rooms)

	for _, room := range rooms {
		result[room.ID] = room
	}
	return result
}

// fetchBanquetDetails fetches banquet hall details for given banquet IDs
func (m *Repository) fetchBanquetDetails(banquetIDs []string) map[string]models.BanquetHall {
	result := make(map[string]models.BanquetHall)
	if len(banquetIDs) == 0 {
		return result
	}

	var banquets []models.BanquetHall
	m.DB.Where("id IN ?", banquetIDs).Find(&banquets)

	for _, banquet := range banquets {
		result[fmt.Sprintf("%d", banquet.ID)] = banquet
	}
	return result
}

// fetchCateringDetails fetches catering menu details for given catering IDs
func (m *Repository) fetchCateringDetails(cateringIDs []string) map[string]models.CateringMenu {
	result := make(map[string]models.CateringMenu)
	if len(cateringIDs) == 0 {
		return result
	}

	var caterings []models.CateringMenu
	m.DB.Where("id IN ?", cateringIDs).Find(&caterings)

	for _, catering := range caterings {
		result[fmt.Sprintf("%d", catering.ID)] = catering
	}
	return result
}
