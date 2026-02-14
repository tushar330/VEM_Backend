package routes

import (
	"github.com/akashtripathi12/TBO_Backend/internal/config"
	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/middleware"
	"github.com/gofiber/fiber/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, cfg *config.Config, repo *handlers.Repository) {

	// --- Health Route ---
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"env":    cfg.Env,
		})
	})

	// API v1 group
	api := app.Group("/api/v1")

	// -----------------------------
	// Public Routes
	// -----------------------------
	api.Post("/auth/signup", repo.SignupHandler)
	api.Post("/auth/login", repo.LoginHandler)
	api.Post("/agents/onboarding", repo.SignupHandler) // legacy support

	// -----------------------------
	// Protected Routes (ENV Based)
	// -----------------------------
	var protected fiber.Router

	if cfg.Env == "development" {
		// Disable auth in development
		protected = api.Group("/")
	} else {
		// Enable auth in production
		protected = api.Group("/", middleware.Protected)
	}

	// Authenticated user
	protected.Get("/me", repo.GetMe)

	// Dashboard
	protected.Get("/dashboard/metrics", repo.GetMetrics)

	// -----------------------------
	// Event Routes
	// -----------------------------
	events := protected.Group("/events")
	events.Get("/", repo.GetEvents)
	events.Post("/", repo.CreateEvent)
	events.Get("/:id", repo.GetEvent)
	events.Put("/:id", repo.UpdateEvent)
	events.Delete("/:id", repo.DeleteEvent)
	events.Get("/:id/venues", repo.GetEventVenues)
	events.Get("/:id/allocations", repo.GetEventAllocations)
	events.Get("/:id/guests", repo.GetGuests)
	events.Post("/:id/head-guest", repo.AssignHeadGuest)

	// -----------------------------
	// Guest Routes
	// -----------------------------
	guests := protected.Group("/guests")
	guests.Post("/", repo.CreateGuest)
	guests.Get("/:id", repo.GetGuest)
	guests.Patch("/:id", repo.UpdateGuest)
	guests.Delete("/:id", repo.DeleteGuest)
	guests.Post("/:id/subguests", repo.AddSubGuest)

	// -----------------------------
	// Allocation Routes
	// -----------------------------
	allocations := protected.Group("/allocations")
	allocations.Post("/", repo.CreateAllocation)
	allocations.Put("/:id", repo.UpdateAllocation)

	// -----------------------------
	// Location Routes (Public)
	// -----------------------------
	locations := api.Group("/locations")
	locations.Get("/countries", repo.GetCountries)
	locations.Get("/cities", repo.GetCities)
	// -----------------------------
	// Hotel Routes (Public)
	// -----------------------------
	hotels := api.Group("/hotels")
	hotels.Get("/", repo.GetHotelsByCity)
	hotels.Get("/:hotelCode/rooms", repo.GetRoomsByHotel)
}
