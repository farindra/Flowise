package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Throttle governs how fast a campaign sends. Every field is user-overridable
// per campaign; the env values below are the defaults, and the separate
// hard ceilings further down are the ones a UI user cannot raise.
//
// Defaults are deliberately conservative: whatsmeow is an unofficial WhatsApp
// client, and bursty sending to non-contacts is the clearest automation
// fingerprint there is.
type Throttle struct {
	MinDelaySec        int    `json:"min_delay_sec"`
	MaxDelaySec        int    `json:"max_delay_sec"`
	BatchSize          int    `json:"batch_size"`
	BatchPauseSec      int    `json:"batch_pause_sec"`
	DailyCap           int    `json:"daily_cap"`
	MaxAttempts        int    `json:"max_attempts"`
	RetryBackoffSec    int    `json:"retry_backoff_sec"`
	RetryBackoffMaxSec int    `json:"retry_backoff_max_sec"`
	HoursStart         string `json:"hours_start"`
	HoursEnd           string `json:"hours_end"`
	WorkingDays        []int  `json:"working_days"` // ISO weekday, 1=Mon .. 7=Sun
	Timezone           string `json:"timezone"`
	CooldownDays       int    `json:"cooldown_days"`
	WarmupFirstN       int    `json:"warmup_first_n"`
}

// Hard ceilings — env-only. This is how "every parameter is configurable"
// coexists with a guardrail: a server admin can raise these in .env, a UI user
// cannot.
var (
	hardDailyCap     = 400
	minDelayFloorSec = 15
	maxRecipients    = 20000
)

var defaultThrottle Throttle

func envInt(key string, def int) int {
	if v := envOr(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func parseWorkingDays(raw string, def []int) []int {
	if raw == "" {
		return def
	}
	out := []int{}
	for _, part := range strings.Split(raw, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 1 && n <= 7 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func initThrottleDefaults() {
	defaultThrottle = Throttle{
		MinDelaySec:        envInt("BROADCAST_MIN_DELAY_SEC", 45),
		MaxDelaySec:        envInt("BROADCAST_MAX_DELAY_SEC", 120),
		BatchSize:          envInt("BROADCAST_BATCH_SIZE", 20),
		BatchPauseSec:      envInt("BROADCAST_BATCH_PAUSE_SEC", 900),
		DailyCap:           envInt("BROADCAST_DAILY_CAP", 200),
		MaxAttempts:        envInt("BROADCAST_MAX_ATTEMPTS", 3),
		RetryBackoffSec:    envInt("BROADCAST_RETRY_BACKOFF_SEC", 300),
		RetryBackoffMaxSec: envInt("BROADCAST_RETRY_BACKOFF_MAX_SEC", 3600),
		HoursStart:         envOr("BROADCAST_HOURS_START", "08:00"),
		HoursEnd:           envOr("BROADCAST_HOURS_END", "17:00"),
		WorkingDays:        parseWorkingDays(envOr("BROADCAST_WORKING_DAYS", ""), []int{1, 2, 3, 4, 5, 6}),
		Timezone:           envOr("BROADCAST_TZ", "Asia/Jakarta"),
		CooldownDays:       envInt("BROADCAST_COOLDOWN_DAYS", 7),
		WarmupFirstN:       envInt("BROADCAST_WARMUP_FIRST_N", 10),
	}
	hardDailyCap = envInt("BROADCAST_HARD_DAILY_CAP", 400)
	minDelayFloorSec = envInt("BROADCAST_MIN_DELAY_FLOOR_SEC", 15)
	maxRecipients = envInt("BROADCAST_MAX_RECIPIENTS", 20000)
}

// mergeThrottle layers a campaign's overrides onto the global defaults and
// applies the env-only ceilings. Only keys the user actually set are stored per
// campaign, so raising a default in .env lifts every campaign that never
// overrode it.
func mergeThrottle(over map[string]any) Throttle {
	return mergeThrottleRaw(over).clamp()
}

// mergeThrottleRaw merges without clamping, so validation can report what the
// user actually asked for instead of the silently-corrected value.
func mergeThrottleRaw(over map[string]any) Throttle {
	t := defaultThrottle
	if over == nil {
		return t
	}
	getInt := func(key string, dst *int) {
		if v, ok := over[key]; ok {
			switch n := v.(type) {
			case float64:
				*dst = int(n)
			case int:
				*dst = n
			case string:
				if parsed, err := strconv.Atoi(n); err == nil {
					*dst = parsed
				}
			}
		}
	}
	getStr := func(key string, dst *string) {
		if v, ok := over[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				*dst = s
			}
		}
	}
	getInt("min_delay_sec", &t.MinDelaySec)
	getInt("max_delay_sec", &t.MaxDelaySec)
	getInt("batch_size", &t.BatchSize)
	getInt("batch_pause_sec", &t.BatchPauseSec)
	getInt("daily_cap", &t.DailyCap)
	getInt("max_attempts", &t.MaxAttempts)
	getInt("retry_backoff_sec", &t.RetryBackoffSec)
	getInt("retry_backoff_max_sec", &t.RetryBackoffMaxSec)
	getInt("cooldown_days", &t.CooldownDays)
	getInt("warmup_first_n", &t.WarmupFirstN)
	getStr("hours_start", &t.HoursStart)
	getStr("hours_end", &t.HoursEnd)
	getStr("timezone", &t.Timezone)
	if v, ok := over["working_days"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			days := []int{}
			for _, d := range arr {
				if f, ok := d.(float64); ok && f >= 1 && f <= 7 {
					days = append(days, int(f))
				}
			}
			if len(days) > 0 {
				t.WorkingDays = days
			}
		}
	}
	return t
}

// clamp applies the env-only ceilings. Called after merge so a campaign can
// never exceed them regardless of what was stored.
func (t Throttle) clamp() Throttle {
	if t.MinDelaySec < minDelayFloorSec {
		t.MinDelaySec = minDelayFloorSec
	}
	if t.MaxDelaySec < t.MinDelaySec {
		t.MaxDelaySec = t.MinDelaySec
	}
	if t.DailyCap > hardDailyCap {
		t.DailyCap = hardDailyCap
	}
	if t.DailyCap < 1 {
		t.DailyCap = 1
	}
	if t.BatchSize < 1 {
		t.BatchSize = 1
	}
	if t.MaxAttempts < 1 {
		t.MaxAttempts = 1
	}
	if t.MaxAttempts > 5 {
		t.MaxAttempts = 5
	}
	if len(t.WorkingDays) == 0 {
		t.WorkingDays = []int{1, 2, 3, 4, 5, 6}
	}
	return t
}

// validateThrottleOverrides rejects nonsense before it is stored, with messages
// in the same Indonesian style as the rest of the CRM API.
func validateThrottleOverrides(over map[string]any) error {
	t := mergeThrottleRaw(over)
	if t.MinDelaySec < minDelayFloorSec {
		return fmt.Errorf("jeda minimum tidak boleh di bawah %d detik", minDelayFloorSec)
	}
	if t.MaxDelaySec < t.MinDelaySec {
		return fmt.Errorf("jeda maksimum harus >= jeda minimum")
	}
	if t.BatchSize < 1 {
		return fmt.Errorf("ukuran batch minimal 1")
	}
	if t.DailyCap > hardDailyCap {
		return fmt.Errorf("batas harian maksimal %d (diatur server)", hardDailyCap)
	}
	if t.DailyCap < 1 {
		return fmt.Errorf("batas harian minimal 1")
	}
	if t.MaxAttempts < 1 || t.MaxAttempts > 5 {
		return fmt.Errorf("jumlah percobaan harus antara 1 dan 5")
	}
	start, err := parseClock(t.HoursStart)
	if err != nil {
		return fmt.Errorf("jam mulai tidak valid (format HH:MM)")
	}
	end, err := parseClock(t.HoursEnd)
	if err != nil {
		return fmt.Errorf("jam selesai tidak valid (format HH:MM)")
	}
	if start >= end {
		return fmt.Errorf("jam mulai harus lebih awal dari jam selesai")
	}
	if len(t.WorkingDays) == 0 {
		return fmt.Errorf("hari kerja tidak boleh kosong")
	}
	if _, err := time.LoadLocation(t.Timezone); err != nil {
		return fmt.Errorf("zona waktu tidak dikenal: %s", t.Timezone)
	}
	return nil
}

// parseClock converts "HH:MM" into minutes past midnight.
func parseClock(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad clock")
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("bad clock")
	}
	return h*60 + m, nil
}

// location resolves the campaign timezone, falling back to UTC rather than
// failing a send.
func (t Throttle) location() *time.Location {
	loc, err := time.LoadLocation(t.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// withinWorkingWindow reports whether `now` falls inside the configured days
// and hours.
func (t Throttle) withinWorkingWindow(now time.Time) bool {
	isoDay := int(now.Weekday())
	if isoDay == 0 {
		isoDay = 7 // time.Sunday is 0; ISO calls it 7
	}
	ok := false
	for _, d := range t.WorkingDays {
		if d == isoDay {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	start, err1 := parseClock(t.HoursStart)
	end, err2 := parseClock(t.HoursEnd)
	if err1 != nil || err2 != nil {
		return true // misconfigured window should not block sending entirely
	}
	mins := now.Hour()*60 + now.Minute()
	return mins >= start && mins < end
}

// estimateSeconds projects how long a campaign of n recipients will take, for
// the composer's "selesai ±hari ini 15:20" hint.
func (t Throttle) estimateSeconds(n int) int {
	if n <= 0 {
		return 0
	}
	avgDelay := (t.MinDelaySec + t.MaxDelaySec) / 2
	total := (n - 1) * avgDelay
	if t.BatchSize > 0 {
		total += ((n - 1) / t.BatchSize) * t.BatchPauseSec
	}
	return total
}
