package main

import (
	"log"
	"os"

	"github.com/akashtripathi12/TBO_Backend/internal/config"
	"github.com/akashtripathi12/TBO_Backend/internal/handlers"
	"github.com/akashtripathi12/TBO_Backend/internal/routes"
	"github.com/akashtripathi12/TBO_Backend/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// Load Config
	cfg := config.Load()

	// Initialize Store
	store.InitDB()
	log.Println("✅ DB Connected. URL:", os.Getenv("DATABASE_URL"))

	// Initialize Repository
	repo := handlers.NewRepository(cfg, store.DB)

	app := fiber.New()

	// Enable CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // For development
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	// Setup Routes
	routes.SetupRoutes(app, cfg, repo)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("TBO Backend Operational 🚀")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(app.Listen(":" + port))
}
