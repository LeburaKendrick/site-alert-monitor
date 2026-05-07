// main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// --- Types ---

type SensorReading struct {
	SiteID      string  `json:"site_id"`
	SensorType  string  `json:"sensor_type"` // "wind", "gas", "temperature", "vibration"
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Timestamp   string  `json:"timestamp"`
}

type Alert struct {
	ID          string  `json:"id"`
	SiteID      string  `json:"site_id"`
	SensorType  string  `json:"sensor_type"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Severity    string  `json:"severity"` // "warning", "critical"
	Message     string  `json:"message"`
	Timestamp   string  `json:"timestamp"`
}

// --- Threshold config (ISO 45001 / OSHA aligned) ---
var thresholds = map[string]struct{ Warning, Critical float64 }{
	"wind":        {Warning: 25.0, Critical: 40.0},  // km/h
	"gas":         {Warning: 10.0, Critical: 25.0},  // % LEL
	"temperature": {Warning: 35.0, Critical: 45.0},  // °C
	"vibration":   {Warning: 5.0,  Critical: 10.0},  // m/s²
}

// --- In-memory store ---
var (
	alerts   []Alert
	alertsMu sync.RWMutex
	clients  = make(map[*websocket.Conn]bool)
	clientMu sync.Mutex
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// --- Alert evaluation ---
func evaluate(reading SensorReading) *Alert {
	limits, ok := thresholds[reading.SensorType]
	if !ok {
		return nil
	}

	var severity, message string
	switch {
	case reading.Value >= limits.Critical:
		severity = "critical"
		message = fmt.Sprintf("CRITICAL: %s reading %.1f %s on site %s exceeds critical threshold (%.1f). Evacuate immediately.",
			reading.SensorType, reading.Value, reading.Unit, reading.SiteID, limits.Critical)
	case reading.Value >= limits.Warning:
		severity = "warning"
		message = fmt.Sprintf("WARNING: %s reading %.1f %s on site %s exceeds warning threshold (%.1f). Investigate.",
			reading.SensorType, reading.Value, reading.Unit, reading.SiteID, limits.Warning)
	default:
		return nil
	}

	return &Alert{
		ID:         fmt.Sprintf("ALT-%d", time.Now().UnixNano()),
		SiteID:     reading.SiteID,
		SensorType: reading.SensorType,
		Value:      reading.Value,
		Threshold:  limits.Warning,
		Severity:   severity,
		Message:    message,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
}

// --- Broadcast to all WebSocket clients ---
func broadcast(alert Alert) {
	payload, err := json.Marshal(alert)
	if err != nil {
		log.Println("Marshal error:", err)
		return
	}

	clientMu.Lock()
	defer clientMu.Unlock()

	for conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Println("WS write error:", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

// --- HTTP Handlers ---

// POST /readings — ingest sensor data
func handleReading(w http.ResponseWriter, r *http.Request) {
	var reading SensorReading
	if err := json.NewDecoder(r.Body).Decode(&reading); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	reading.Timestamp = time.Now().UTC().Format(time.RFC3339)

	alert := evaluate(reading)
	if alert != nil {
		alertsMu.Lock()
		alerts = append(alerts, *alert)
		alertsMu.Unlock()

		go broadcast(*alert)

		log.Printf("[ALERT][%s] %s", alert.Severity, alert.Message)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"status": "alert_raised", "alert": alert})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Reading within safe limits"})
}

// GET /alerts — retrieve all logged alerts
func handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")

	alertsMu.RLock()
	defer alertsMu.RUnlock()

	filtered := []Alert{}
	for _, a := range alerts {
		if severity == "" || a.Severity == severity {
			filtered = append(filtered, a)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"count":  len(filtered),
		"alerts": filtered,
	})
}

// GET /ws — WebSocket endpoint for real-time alerts
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}
	defer conn.Close()

	clientMu.Lock()
	clients[conn] = true
	clientMu.Unlock()

	log.Printf("New WebSocket client connected. Total: %d", len(clients))

	// Keep connection alive, clean up on disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			clientMu.Lock()
			delete(clients, conn)
			clientMu.Unlock()
			log.Printf("Client disconnected. Total: %d", len(clients))
			break
		}
	}
}

// GET /health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	alertsMu.RLock()
	total := len(alerts)
	alertsMu.RUnlock()

	json.NewEncoder(w).Encode(map[string]any{
		"status":       "running",
		"total_alerts": total,
		"clients":      len(clients),
		"uptime":       time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Main ---
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/readings", handleReading)
	mux.HandleFunc("/alerts", handleGetAlerts)
	mux.HandleFunc("/ws", handleWebSocket)
	mux.HandleFunc("/health", handleHealth)

	log.Println("Site Alert Monitor starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
