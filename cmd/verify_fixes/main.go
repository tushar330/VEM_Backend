package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	BaseURL    = "http://localhost:8080/api/v1"
	AgentEmail = "agent@test.com"
	AgentPass  = "Test@123"

	// IDs from previous seed run
	EventMumbaiActive    = "5d4e5c9f-6029-4481-bd24-6330daf4d0e1"
	EventJaipurFinalized = "291df02b-fc8a-4eca-892d-32cc5e5e3f56"
)

func main() {
	log.Println("🚀 Starting Verification Script...")

	// 1. Login
	token, err := login(AgentEmail, AgentPass)
	if err != nil {
		log.Fatalf("❌ Login failed: %v", err)
	}
	log.Println("✅ Login successful")

	// 2. Verify Finalized Event Locks (Jaipur)
	log.Printf("\n--- Test 1: Verify Guard on Finalized Event (%s) ---", EventJaipurFinalized)
	verifyGuard(token, EventJaipurFinalized, "Auto-Allocate", "/events/"+EventJaipurFinalized+"/auto-allocate")

	// 3. Verify Reopen (Jaipur)
	log.Printf("\n--- Test 2: Verify Reopen (%s) ---", EventJaipurFinalized)
	verifyAction(token, EventJaipurFinalized, "Reopen", "/events/"+EventJaipurFinalized+"/reopen", 200)

	// 4. Verify Active Event Finalize (Mumbai)
	log.Printf("\n--- Test 3: Verify Finalize (%s) ---", EventMumbaiActive)
	verifyAction(token, EventMumbaiActive, "Finalize", "/events/"+EventMumbaiActive+"/finalize", 200)

	// 5. Verify Guard on Newly Finalized Event (Mumbai)
	log.Printf("\n--- Test 4: Verify Guard on Newly Finalized Event (%s) ---", EventMumbaiActive)
	verifyGuard(token, EventMumbaiActive, "Auto-Allocate", "/events/"+EventMumbaiActive+"/auto-allocate")

	log.Println("\n✅ Verification Complete!")
}

func login(email, password string) (string, error) {
	data := map[string]string{"email": email, "password": password}
	jsonData, _ := json.Marshal(data)

	resp, err := http.Post(BaseURL+"/auth/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	token, ok := result["token"].(string)
	if !ok {
		// Try nested data
		if data, ok := result["data"].(map[string]interface{}); ok {
			if t, ok := data["token"].(string); ok {
				return t, nil
			}
		}
		return "", fmt.Errorf("token not found in response")
	}
	return token, nil
}

func verifyGuard(token, eventID, actionName, endpoint string) {
	req, _ := http.NewRequest("POST", BaseURL+endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ %s request failed: %v", actionName, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 400 && strings.Contains(string(body), "finalized") {
		log.Printf("✅ Success: %s blocked as expected (400 Bad Request). Response: %s", actionName, string(body))
	} else {
		log.Printf("❌ Failure: %s NOT blocked. Status: %d. Response: %s", actionName, resp.StatusCode, string(body))
	}
}

func verifyAction(token, eventID, actionName, endpoint string, expectedStatus int) {
	req, _ := http.NewRequest("POST", BaseURL+endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("❌ %s request failed: %v", actionName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == expectedStatus {
		log.Printf("✅ Success: %s executed. Status: %d", actionName, resp.StatusCode)
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Failure: %s failed. Status: %d. Response: %s", actionName, resp.StatusCode, string(body))
	}
}
