package handlers

import (
	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/store"
	"github.com/gofiber/fiber/v2"
)

// GetRoomsByHotel fetches all available room offers for a specific hotel
// GET /api/v1/hotels/:hotelCode/rooms
func (r *Repository) GetRoomsByHotel(c *fiber.Ctx) error {
	hotelCode := c.Params("hotelCode")

	// 1. Validate Input
	if hotelCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "error",
			"message": "hotelCode parameter is required",
		})
	}

	var rooms []models.RoomOffer

	// 2. Query Database
	// We SELECT * FROM room_offers WHERE hotel_id = ?
	result := store.DB.Where("hotel_id = ?", hotelCode).Find(&rooms)

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"status":  "error",
			"message": "Failed to fetch rooms",
		})
	}

	// 3. Return Response
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "success",
		"count":  len(rooms),
		"data":   rooms,
	})
}
