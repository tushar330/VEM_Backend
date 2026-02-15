package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// CreateAllocation wraps the AllocateFamilyHandler for Repository pattern
func (repo *Repository) CreateAllocation(c *fiber.Ctx) error {
	handler := AllocateFamilyHandler(repo.DB)
	return handler(c)
}

// UpdateAllocation handles updating an allocation
func (repo *Repository) UpdateAllocation(c *fiber.Ctx) error {
	handler := UpdateAllocationHandler(repo.DB)
	return handler(c)
}

// GetEventAllocations handles retrieving allocations for an event
func (repo *Repository) GetEventAllocations(c *fiber.Ctx) error {
	handler := GetEventAllocationsHandler(repo.DB)
	return handler(c)
}

// AutoAllocate handles auto-allocation of families
func (repo *Repository) AutoAllocate(c *fiber.Ctx) error {
	handler := AutoAllocateHandler(repo.DB)
	return handler(c)
}
