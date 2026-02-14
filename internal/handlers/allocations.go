package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// CreateAllocation wraps the AllocateFamilyHandler for Repository pattern
func (repo *Repository) CreateAllocation(c *fiber.Ctx) error {
	handler := AllocateFamilyHandler(repo.DB)
	return handler(c)
}

// UpdateAllocation handles updating an allocation (placeholder for future implementation)
func (repo *Repository) UpdateAllocation(c *fiber.Ctx) error {
	// TODO: Implement allocation update logic
	// For now, return method not implemented
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "Update allocation not yet implemented",
	})
}
