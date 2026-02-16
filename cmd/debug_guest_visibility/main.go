package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/akashtripathi12/TBO_Backend/internal/models"
	"github.com/akashtripathi12/TBO_Backend/internal/utils"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// 1. Setup DB to check raw data first
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	eventID := "7e930542-4c3c-4c1a-91ea-c193116acedc"

	// Check raw DB count
	var count int64
	db.Model(&models.Guest{}).Where("event_id = ?", eventID).Count(&count)
	fmt.Printf("DB Guest Count for %s: %d\n", eventID, count)

	// 2. Generate Token for Agent
	var agent models.User
	// We used verification.agent@tbo.com in seeding
	if err := db.Where("email = ?", "verification.agent@tbo.com").First(&agent).Error; err != nil {
		log.Fatal("Agent not found:", err)
	}
	token, _ := utils.GenerateToken(agent.ID, agent.Email, agent.Role)

	// 3. Make HTTP Request to API (Running on localhost:8080)
	// We assume the server is running. If not, this part fails.
	client := &http.Client{Timeout: 10 * time.Second}

	fetch := func(endpoint string) string {
		url := "http://localhost:8080/api/v1" + endpoint
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching %s: %v", url, err)
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Sprintf("Response for %s (Status %d):\n%s\n", endpoint, resp.StatusCode, string(body))
	}

	fmt.Println(fetch("/events/" + eventID + "/guests"))
	fmt.Println(fetch("/events/" + eventID + "/allocations"))
}
