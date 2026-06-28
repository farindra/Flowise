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

// POST /api/zones/{id}/report — kirim laporan zona ke Telegram dan/atau email
func handleSendReport(w http.ResponseWriter, r *http.Request) {
	zoneID := r.PathValue("id")

	var body struct {
		Channels []string `json:"channels"` // ["telegram","email"]
		Email    string   `json:"email"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Ambil zona
	zones, err := dbListZones(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var zone *Zone
	for i := range zones {
		if zones[i].ID == zoneID {
			zone = &zones[i]
			break
		}
	}
	if zone == nil {
		jsonError(w, "zone not found", http.StatusNotFound)
		return
	}

	latest, _ := dbLatestReadings(r.Context(), zoneID)
	zone.Latest = latest
	zone.OpenAlerts = dbCountOpenAlerts(r.Context(), zoneID)

	openAlerts, _ := dbListAlerts(r.Context(), true)
	var zoneAlerts []Alert
	for _, a := range openAlerts {
		if a.ZoneID == zoneID {
			zoneAlerts = append(zoneAlerts, a)
		}
	}

	text := buildZoneReport(zone, zoneAlerts)

	results := map[string]string{}

	for _, ch := range body.Channels {
		switch ch {
		case "telegram":
			if err := sendTelegramAlert(r.Context(), text); err != nil {
				results["telegram"] = "error: " + err.Error()
			} else {
				results["telegram"] = "sent"
			}
		case "email":
			results["email"] = "not_configured"
		}
	}

	jsonOK(w, map[string]any{"report": text, "results": results})
}

func buildZoneReport(zone *Zone, alerts []Alert) string {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	ts := now.Format("Mon, 02 Jan 2006 — 15:04 WIB")

	statusEmoji := "✅"
	if zone.OpenAlerts > 0 {
		statusEmoji = "🚨"
	}

	text := statusEmoji + " *Laporan Kondisi Zona — AAMG*\n\n"
	text += "📍 *" + zone.Name + "*\n"
	text += "🕐 " + ts + "\n\n"
	text += "*Sensor Terkini:*\n"

	sensors := []struct{ key, label, unit, emoji string }{
		{"soil_humidity", "Kelembaban Tanah", "%", "🪴"},
		{"air_humidity", "Kelembaban Udara", "%", "💨"},
		{"temperature", "Suhu Udara", "°C", "🌡️"},
	}
	for _, s := range sensors {
		if r, ok := zone.Latest[s.key]; ok {
			text += s.emoji + " " + s.label + ": " + formatFloat(r.Value) + s.unit + "\n"
		} else {
			text += s.emoji + " " + s.label + ": —\n"
		}
	}

	if len(alerts) == 0 {
		text += "\n✅ *Tidak ada alert aktif*\n"
	} else {
		text += "\n⚠️ *" + itoa(len(alerts)) + " Alert Aktif:*\n"
		for _, a := range alerts {
			label := sensorLabels[a.SensorType]
			unit := sensorUnits[a.SensorType]
			var cond string
			if a.Direction == "below" {
				cond = "min " + formatFloat(a.Threshold) + unit
			} else {
				cond = "max " + formatFloat(a.Threshold) + unit
			}
			text += "• " + label + ": " + formatFloat(a.Value) + unit + " (" + cond + ")\n"
		}
	}

	text += "\n_Dikirim dari IoT Monitor AAMG_"
	return text
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return itoa(int(f))
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}

func itoa(n int) string {
	return strconv.Itoa(n)
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
