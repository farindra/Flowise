package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func apiAuth(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("X-Internal-Key")
		if key != "" && k != key {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// GET /api/zones — semua zona + latest reading + status + alert count
func handleListZones(w http.ResponseWriter, r *http.Request) {
	zones, err := dbListZones(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range zones {
		latest, _ := dbLatestReadings(r.Context(), zones[i].ID)
		zones[i].Latest = latest
		zones[i].OpenAlerts = dbCountOpenAlerts(r.Context(), zones[i].ID)
		if zones[i].OpenAlerts > 0 {
			zones[i].Status = "alert"
		} else {
			zones[i].Status = "ok"
		}
	}
	if zones == nil {
		zones = []Zone{}
	}
	jsonOK(w, zones)
}

// POST /api/readings — terima data dari sensor / MQTT bridge
// Body: {"zone_id":"zona-a","sensor_type":"soil_humidity","value":35.2,"unit":"%"}
// atau array: [{"zone_id":...}, ...]
func handlePostReading(w http.ResponseWriter, r *http.Request) {
	// support single atau array
	var singles []struct {
		ZoneID     string    `json:"zone_id"`
		SensorType string    `json:"sensor_type"`
		Value      float64   `json:"value"`
		Unit       string    `json:"unit"`
		RecordedAt time.Time `json:"recorded_at"`
	}

	dec := json.NewDecoder(r.Body)
	// Try array first
	if err := dec.Decode(&singles); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	// fetch zones once for threshold check
	zones, _ := dbListZones(r.Context())
	zoneMap := map[string]*Zone{}
	for i := range zones {
		zoneMap[zones[i].ID] = &zones[i]
	}

	saved := 0
	for _, s := range singles {
		if s.ZoneID == "" || s.SensorType == "" {
			continue
		}
		if s.RecordedAt.IsZero() {
			s.RecordedAt = time.Now().UTC()
		}
		reading := &Reading{
			ZoneID:     s.ZoneID,
			SensorType: s.SensorType,
			Value:      s.Value,
			Unit:       s.Unit,
			RecordedAt: s.RecordedAt,
		}
		if err := dbInsertReading(r.Context(), reading); err == nil {
			saved++
			if z, ok := zoneMap[s.ZoneID]; ok {
				checkThresholds(r.Context(), z.ID, z.Name, z.Thresholds, reading)
			}
		}
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]int{"saved": saved})
}

// GET /api/alerts — daftar alert (default: hanya open)
func handleListAlerts(w http.ResponseWriter, r *http.Request) {
	onlyOpen := r.URL.Query().Get("resolved") != "true"
	alerts, err := dbListAlerts(r.Context(), onlyOpen)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []Alert{}
	}
	jsonOK(w, alerts)
}

// GET /api/readings/history?zone_id=zona-a&sensor_type=soil_humidity&hours=24
func handleReadingsHistory(w http.ResponseWriter, r *http.Request) {
	zoneID := r.URL.Query().Get("zone_id")
	sensorType := r.URL.Query().Get("sensor_type")
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		hours, _ = strconv.Atoi(h)
	}
	if zoneID == "" || sensorType == "" {
		jsonError(w, "zone_id and sensor_type required", http.StatusBadRequest)
		return
	}
	readings, err := dbReadingsHistory(r.Context(), zoneID, sensorType, hours)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if readings == nil {
		readings = []Reading{}
	}
	jsonOK(w, readings)
}

// PUT /api/alerts/{id}/resolve
func handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := dbResolveAlerts(r.Context(), "", ""); err != nil {
		_ = err
	}
	// resolve specific alert
	_, err := pool.Exec(r.Context(), `UPDATE iot_alerts SET resolved=TRUE WHERE id=$1`, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "resolved"})
}
