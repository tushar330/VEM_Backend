package routes

import (
	"github.com/akashtripathi12/TBO_Backend/internal/config"
	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/middleware"

	// "github.com/akashtripathi12/TBO_Backend/internal/middleware"
	"github.com/gofiber/fiber/v2"
)

// SetupRoutes configures all application routes
func SetupRoutes(app *fiber.App, cfg *config.Config, repo *handlers.Repository) {

	// --- Health Route ---

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

	protected = api.Group("/", middleware.Protected)

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
	events.Post("/:id/guests", repo.CreateGuest)
	events.Post("/:id/auto-allocate", repo.AutoAllocate)
	events.Get("/:id/guests", repo.GetGuests)
	events.Post("/:id/event-manager", repo.AssignEventManager)
	events.Post("/:id/send-invites", repo.SendInvites) // New route
	events.Post("/:id/finalize", repo.FinalizeRooms)
	events.Post("/:id/reopen", repo.ReopenAllocation)
	events.Post("/:id/ai-allocate", repo.AIAllocate) // Read-only AI suggestion engine

	// Cart Routes
	events.Get("/:id/cart", repo.GetEventCart)
	events.Post("/:id/cart", repo.AddToCart)
	events.Post("/:id/cart/bulk", repo.BulkAddToCart)
	events.Patch("/:id/cart/:cartItemId", repo.UpdateCartItem)
	events.Delete("/:id/cart/:cartItemId", repo.RemoveFromCart)
	events.Delete("/:id/cart/hotel/:hotelId", repo.RemoveHotelGroupFromCart)
	events.Post("/:id/cart/approve", repo.UpdateCartStatus)

	// -----------------------------
	// Guest Routes
	// -----------------------------
	guests := protected.Group("/guests")
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
	// Flight Routes
	// -----------------------------
	// Event-specific	// --- Global Flight Routes (Public) ---
	flights := protected.Group("/flights")
	flights.Get("/locations", repo.GetFlightLocations) // Get unique booking locations
	flights.Get("/", repo.GetAllFlights)               // List all global flights
	flights.Get("/:id", repo.GetFlight)                // Get single flight
	flights.Post("/", repo.CreateFlight)               // Create flight (admin)
	flights.Put("/:id", repo.UpdateFlight)             // Update flight (admin)
	flights.Delete("/:id", repo.DeleteFlight)          // Delete flight (admin)

	// --- Global Transfer Routes (Public) ---
	transfers := protected.Group("/transfers")
	transfers.Get("/", repo.GetAllTransfers)      // List all global transfers
	transfers.Get("/:id", repo.GetTransfer)       // Get single transfer
	transfers.Post("/", repo.CreateTransfer)      // Create transfer (admin)
	transfers.Put("/:id", repo.UpdateTransfer)    // Update transfer (admin)
	transfers.Delete("/:id", repo.DeleteTransfer) // Delete transfer (admin)

	// --- Event-Specific Flight Booking Routes ---
	eventFlights := events.Group("/:id/flights")
	eventFlights.Get("/", repo.GetEventFlightBookings)            // Get event's flight bookings
	eventFlights.Post("/book", repo.BookFlightForEvent)           // Book a flight for event
	eventFlights.Delete("/:booking_id", repo.CancelFlightBooking) // Cancel flight booking

	// --- Event-Specific Transfer Booking Routes ---
	eventTransfers := events.Group("/:id/transfers")
	eventTransfers.Get("/", repo.GetEventTransferBookings)            // Get event's transfer bookings
	eventTransfers.Post("/book", repo.BookTransferForEvent)           // Book a transfer for event
	eventTransfers.Delete("/:booking_id", repo.CancelTransferBooking) // Cancel transfer booking

	// -----------------------------
	// Location Routes (Public)
	// -----------------------------
	locations := api.Group("/locations")
	locations.Get("/countries", repo.GetCountries)
	locations.Get("/cities", repo.GetCities)

	// -----------------------------
	// Hotel Routes (Public)
	// -----------------------------
	hotels := protected.Group("/hotels")
	hotels.Get("/", repo.GetHotelsByCity)
	hotels.Get("/:id", repo.GetHotel)
	hotels.Get("/:hotelCode/rooms", repo.GetRoomsByHotel)
	hotels.Get("/:hotelCode/banquets", repo.GetBanquetsByHotel)
	hotels.Get("/:hotelCode/catering", repo.GetCateringByHotel)

	// -----------------------------
	// Negotiation Routes
	// -----------------------------
	negotiation := api.Group("/negotiation")
	negotiation.Post("/init", middleware.Protected, repo.StartNegotiation)
	negotiation.Post("/counter", repo.SubmitCounterOffer) // Public access via share_token
	negotiation.Get("/:id/diff", repo.GetNegotiationDiff) // Public or Protected? Let's keep public for hotel view
	negotiation.Post("/lock", middleware.Protected, repo.LockDeal)
	negotiation.Get("/token/:token", repo.GetNegotiationByToken) // Resolve share_token to session

	// --- TBO Admin Routes ---
	admin := protected.Group("/admin")
	admin.Get("/negotiations", repo.GetAllNegotiations)
}
