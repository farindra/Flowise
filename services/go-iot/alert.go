package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var sensorLabels = map[string]string{
	"soil_humidity": "Kelembaban Tanah",
	"air_humidity":  "Kelembaban Udara",
	"temperature":   "Suhu Udara",
	"motion":        "Gerakan",
}

var sensorUnits = map[string]string{
	"soil_humidity": "%",
	"air_humidity":  "%",
	"temperature":   "°C",
	"motion":        "",
}

func alertEmoji(sensorType, direction string) string {
	switch sensorType {
	case "soil_humidity":
		if direction == "below" {
			return "🏜️"
		}
		return "💧"
	case "temperature":
		if direction == "above" {
			return "🔥"
		}
		return "❄️"
	case "motion":
		return "⚠️"
	}
	return "🔔"
}

func buildAlertText(a *Alert) string {
	label := sensorLabels[a.SensorType]
	if label == "" {
		label = a.SensorType
	}
	unit := sensorUnits[a.SensorType]
	emoji := alertEmoji(a.SensorType, a.Direction)

	var cond string
	if a.Direction == "below" {
		cond = fmt.Sprintf("%.1f%s (batas minimal %.1f%s)", a.Value, unit, a.Threshold, unit)
	} else {
		cond = fmt.Sprintf("%.1f%s (batas maksimal %.1f%s)", a.Value, unit, a.Threshold, unit)
	}

	return fmt.Sprintf(
		"%s *ALERT LAHAN — AAMG*\n\n📍 *Zona:* %s\n📊 *Sensor:* %s\n📉 *Nilai:* %s\n\n%s\n\n_Segera lakukan pengecekan lapangan._",
		emoji, a.ZoneName, label, cond, a.Message,
	)
}

func buildRecoveryText(zoneName, sensorType string, value float64) string {
	label := sensorLabels[sensorType]
	unit := sensorUnits[sensorType]
	return fmt.Sprintf("✅ *RECOVERY — %s*\n📍 %s\n📊 %s kembali normal: %.1f%s",
		"AAMG Lahan", zoneName, label, value, unit)
}

func sendTelegramAlert(ctx context.Context, text string) error {
	botToken := os.Getenv("TELEGRAM_ALERT_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_ALERT_CHAT_ID")
	if botToken == "" || chatID == "" {
		return nil // tidak dikonfigurasi, skip silently
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram API %d", resp.StatusCode)
	}
	return nil
}

// checkThresholds evaluates a new reading against zone thresholds and fires alerts
func checkThresholds(ctx context.Context, zoneID, zoneName string, thresholds map[string]any, r *Reading) {
	th, ok := thresholds[r.SensorType]
	if !ok {
		return
	}
	thMap, ok := th.(map[string]any)
	if !ok {
		return
	}

	unit := sensorUnits[r.SensorType]
	_ = unit

	fired := false

	if minVal, ok := thMap["min"]; ok {
		min := toFloat(minVal)
		if r.Value < min {
			fired = true
			label := sensorLabels[r.SensorType]
			msg := buildAlertMsg(r.SensorType, "below", r.Value, min)
			a := &Alert{
				ZoneID: zoneID, ZoneName: zoneName,
				SensorType: r.SensorType, Value: r.Value, Threshold: min,
				Direction: "below",
				Message:   msg,
			}
			id, err := dbInsertAlert(ctx, a)
			if err == nil {
				a.ID = id
				text := buildAlertText(a)
				if err := sendTelegramAlert(ctx, text); err == nil {
					_ = dbMarkAlertSent(ctx, id)
				}
			}
			_ = label
		}
	}

	if maxVal, ok := thMap["max"]; ok {
		max := toFloat(maxVal)
		if r.Value > max {
			fired = true
			msg := buildAlertMsg(r.SensorType, "above", r.Value, max)
			a := &Alert{
				ZoneID: zoneID, ZoneName: zoneName,
				SensorType: r.SensorType, Value: r.Value, Threshold: max,
				Direction: "above",
				Message:   msg,
			}
			id, err := dbInsertAlert(ctx, a)
			if err == nil {
				a.ID = id
				text := buildAlertText(a)
				if err := sendTelegramAlert(ctx, text); err == nil {
					_ = dbMarkAlertSent(ctx, id)
				}
			}
		}
	}

	if !fired {
		// nilai kembali normal — resolve alert lama dan kirim recovery
		open := dbCountOpenAlerts(ctx, zoneID)
		if open > 0 {
			_ = dbResolveAlerts(ctx, zoneID, r.SensorType)
			text := buildRecoveryText(zoneName, r.SensorType, r.Value)
			_ = sendTelegramAlert(ctx, text)
		}
	}
}

func buildAlertMsg(sensorType, direction string, value, threshold float64) string {
	unit := sensorUnits[sensorType]
	label := sensorLabels[sensorType]
	if label == "" {
		label = sensorType
	}
	parts := []string{}
	switch {
	case sensorType == "soil_humidity" && direction == "below":
		parts = append(parts, fmt.Sprintf("Tanah terlalu kering (%.1f%s). Segera lakukan penyiraman.", value, unit))
	case sensorType == "temperature" && direction == "above":
		parts = append(parts, fmt.Sprintf("Suhu terlalu tinggi (%.1f%s). Periksa kondisi tanaman dan pompa air.", value, unit))
	case sensorType == "air_humidity" && direction == "below":
		parts = append(parts, fmt.Sprintf("Kelembaban udara rendah (%.1f%s). Pertimbangkan pengairan tambahan.", value, unit))
	case sensorType == "motion":
		parts = append(parts, "Terdeteksi gerakan mencurigakan di luar jam operasional. Segera periksa area.")
	default:
		if direction == "below" {
			parts = append(parts, fmt.Sprintf("%s di bawah batas: %.1f%s (min: %.1f%s).", label, value, unit, threshold, unit))
		} else {
			parts = append(parts, fmt.Sprintf("%s melebihi batas: %.1f%s (max: %.1f%s).", label, value, unit, threshold, unit))
		}
	}
	return strings.Join(parts, " ")
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
