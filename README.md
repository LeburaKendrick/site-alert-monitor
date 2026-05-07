# site-alert-monitor

> A high-performance Go service that ingests real-time sensor readings from industrial construction sites, evaluates safety thresholds, and broadcasts live alerts to connected clients via WebSockets.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.21-00ADD8?logo=go)
![WebSockets](https://img.shields.io/badge/WebSockets-gorilla-4A90D9)

---

## Overview

`site-alert-monitor` is a lightweight backend service designed for real-time safety monitoring on scaffolding and industrial construction sites. It receives continuous sensor data (wind speed, gas levels, temperature, vibration), evaluates each reading against configurable safety thresholds, and immediately broadcasts alerts to any connected WebSocket clients — dashboards, mobile apps, or control room displays.

Built with Go's concurrency model (goroutines + mutexes), the service handles multiple simultaneous sensor streams and WebSocket connections efficiently with minimal resource usage.

Thresholds are aligned with **OSHA** and **ISO 45001:2018** industrial safety standards.

---

## Features

- `POST /readings` — ingest a sensor reading; triggers alert if threshold exceeded
- `GET /alerts` — retrieve logged alerts, filterable by severity
- `GET /ws` — WebSocket endpoint for real-time alert streaming
- `GET /health` — service status, total alerts, active client count
- Concurrent WebSocket broadcast using goroutines
- Thread-safe in-memory alert store with `sync.RWMutex`
- Severity levels: `warning` and `critical`
- Configurable per-sensor thresholds (warning + critical)
- Structured JSON throughout

---

## Supported Sensor Types & Thresholds

| Sensor | Warning | Critical | Unit | Standard |
|---|---|---|---|---|
| `wind` | 25 | 40 | km/h | OSHA 1926.502 |
| `gas` | 10 | 25 | % LEL | OSHA 1910.146 |
| `temperature` | 35 | 45 | °C | ISO 45001 §8.1 |
| `vibration` | 5 | 10 | m/s² | ISO 5349-1 |

---

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.21 |
| WebSockets | gorilla/websocket v1.5 |
| Concurrency | goroutines + sync.RWMutex |
| HTTP | net/http (stdlib) |
| Data | In-memory (swap for Redis or Postgres) |

---

## Getting Started

### Prerequisites

- Go 1.21+

### Installation

```bash
git clone https://github.com/your-handle/site-alert-monitor.git
cd site-alert-monitor
go mod tidy
go run main.go
```

Service starts on `http://localhost:8080`

---

## API Reference

### Submit a sensor reading

```bash
curl -X POST http://localhost:8080/readings \
  -H "Content-Type: application/json" \
  -d '{
    "site_id": "Block-A",
    "sensor_type": "wind",
    "value": 42.5,
    "unit": "km/h"
  }'
```

#### Response — alert raised

```json
{
  "status": "alert_raised",
  "alert": {
    "id": "ALT-1712345678901234567",
    "site_id": "Block-A",
    "sensor_type": "wind",
    "value": 42.5,
    "threshold": 25.0,
    "severity": "critical",
    "message": "CRITICAL: wind reading 42.5 km/h on site Block-A exceeds critical threshold (40.0). Evacuate immediately.",
    "timestamp": "2025-03-15T10:22:04Z"
  }
}
```

#### Response — within safe limits

```json
{ "status": "ok", "message": "Reading within safe limits" }
```

---

### Get all alerts

```bash
# All alerts
curl http://localhost:8080/alerts

# Critical only
curl "http://localhost:8080/alerts?severity=critical"
```

---

### Connect via WebSocket

Any WebSocket client connecting to `ws://localhost:8080/ws` will receive alert payloads in real time as sensors breach thresholds.

Example using `websocat`:

```bash
websocat ws://localhost:8080/ws
```

Example browser client:

```js
const ws = new WebSocket("ws://localhost:8080/ws");
ws.onmessage = (event) => {
  const alert = JSON.parse(event.data);
  console.log(`[${alert.severity.toUpperCase()}] ${alert.message}`);
};
```

---

### Health check

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "running",
  "total_alerts": 14,
  "clients": 3,
  "uptime": "2025-03-15T10:00:00Z"
}
```

---

## Architecture

```
Sensor / IoT device
        │
        ▼
POST /readings
        │
   evaluate()  ──── within limits ──▶ 200 OK
        │
   threshold exceeded
        │
   store alert
        │
   broadcast() ──▶ goroutine ──▶ all WebSocket clients
        │
  201 Created + alert payload
```

---

## Project Structure

```
site-alert-monitor/
├── main.go          # All handlers, types, concurrency logic
├── go.mod
├── go.sum
└── README.md
```

---

## Extending

### Add a new sensor type

In `main.go`, add an entry to the `thresholds` map:

```go
var thresholds = map[string]struct{ Warning, Critical float64 }{
    // existing...
    "noise": {Warning: 85.0, Critical: 100.0}, // dB — OSHA 1910.95
}
```

### Persist alerts to a database

Replace the in-memory slice with a PostgreSQL insert:

```go
_, err := db.Exec(
    "INSERT INTO alerts (id, site_id, sensor_type, value, severity, message, timestamp) VALUES ($1,$2,$3,$4,$5,$6,$7)",
    alert.ID, alert.SiteID, alert.SensorType, alert.Value, alert.Severity, alert.Message, alert.Timestamp,
)
```

---

## Roadmap

- [ ] Persist alerts to PostgreSQL
- [ ] Add Redis pub/sub for multi-instance broadcast
- [ ] Configurable thresholds via JSON config file or environment variables
- [ ] Authentication middleware for `/readings` and `/ws`
- [ ] Prometheus metrics endpoint (`/metrics`)
- [ ] Docker + docker-compose setup
- [ ] Integration with `hse-compliance-dashboard` for live feed panel

---

## Background

Built to address a real gap observed during two years of petrochemical site work: safety threshold breaches were often caught too late because monitoring relied on periodic manual inspections rather than continuous data. This service demonstrates how a lightweight Go backend can provide the real-time alerting layer that high-risk industrial sites need, with the concurrency model to handle many sensors simultaneously without degrading performance.

---

## License

MIT
