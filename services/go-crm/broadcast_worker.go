package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// reVar matches {{name}} and {{name|default}}. Plain substitution only — no
// template engine, no user code execution.
var reVar = regexp.MustCompile(`\{\{\s*(\w+)\s*(?:\|([^}]*))?\}\}`)

// renderTemplate substitutes recipient variables. An unknown or empty variable
// falls back to its inline default, or to an empty string.
func renderTemplate(tpl string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(tpl, func(m string) string {
		g := reVar.FindStringSubmatch(m)
		if v := strings.TrimSpace(vars[g[1]]); v != "" {
			return v
		}
		return strings.TrimSpace(g[2])
	})
}

// sessionGate serializes sends per sending identity. Campaigns on different WA
// numbers run in parallel; two campaigns on the same number queue behind each
// other, which is the correct behaviour — parallel sends from one number both
// defeat the throttle maths and are the clearest automation fingerprint.
var (
	gateMu      sync.Mutex
	sessionGate = map[string]*sync.Mutex{}
	inFlight    = map[string]bool{} // broadcast IDs currently running
)

func gateFor(senderID string) *sync.Mutex {
	gateMu.Lock()
	defer gateMu.Unlock()
	if g, ok := sessionGate[senderID]; ok {
		return g
	}
	g := &sync.Mutex{}
	sessionGate[senderID] = g
	return g
}

func markInFlight(id string) bool {
	gateMu.Lock()
	defer gateMu.Unlock()
	if inFlight[id] {
		return false
	}
	inFlight[id] = true
	return true
}

func clearInFlight(id string) {
	gateMu.Lock()
	delete(inFlight, id)
	gateMu.Unlock()
}

// startBroadcastWorker reconciles interrupted sends, then polls for due
// campaigns. Runs in-process: no extra PM2 entry, no new failure mode, and it
// shares the existing pgxpool.
func startBroadcastWorker() {
	ctx := context.Background()

	// A row left in 'sending' means we died between the provider call and the
	// status write. Surface it rather than guessing.
	if n, err := dbReconcileSendingRecipients(ctx); err != nil {
		log.Printf("[broadcast] reconcile error: %v", err)
	} else if n > 0 {
		log.Printf("[broadcast] %d penerima ditandai 'uncertain' setelah restart (perlu ditinjau manual)", n)
	}

	if !envBool("BROADCAST_AUTO_RESUME", true) {
		if _, err := pool.Exec(ctx,
			`UPDATE crm_broadcasts SET status='paused', updated_at=NOW() WHERE status='running'`); err != nil {
			log.Printf("[broadcast] auto-resume disable error: %v", err)
		}
	}

	tick := time.Duration(envInt("BROADCAST_TICK_SEC", 5)) * time.Second
	if tick < time.Second {
		tick = 5 * time.Second
	}

	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for range ticker.C {
			scheduleDueCampaigns(ctx)
		}
	}()

	log.Printf("[broadcast] worker aktif (tick %s, delay %d-%d dtk, cap %d/hari, jam %s-%s %s)",
		tick, defaultThrottle.MinDelaySec, defaultThrottle.MaxDelaySec,
		defaultThrottle.DailyCap, defaultThrottle.HoursStart, defaultThrottle.HoursEnd,
		defaultThrottle.Timezone)
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(envOr(key, "")) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return def
}

func scheduleDueCampaigns(ctx context.Context) {
	campaigns, err := dbDueBroadcasts(ctx)
	if err != nil {
		log.Printf("[broadcast] scan due: %v", err)
		return
	}
	for i := range campaigns {
		b := campaigns[i]

		if b.Status == BStatusScheduled {
			if err := dbStartBroadcast(ctx, b.ID); err != nil {
				log.Printf("[broadcast] promote %s: %v", b.ID, err)
				continue
			}
			b.Status = BStatusRunning
		}

		remaining, err := dbPendingRecipientCount(ctx, b.ID)
		if err != nil {
			log.Printf("[broadcast] count %s: %v", b.ID, err)
			continue
		}
		if remaining == 0 {
			if err := dbFinishBroadcast(ctx, b.ID); err != nil {
				log.Printf("[broadcast] finish %s: %v", b.ID, err)
			}
			continue
		}

		if !markInFlight(b.ID) {
			continue
		}
		go func(camp Broadcast) {
			defer clearInFlight(camp.ID)
			gate := gateFor(camp.SenderID)
			gate.Lock()
			defer gate.Unlock()
			runCampaign(context.Background(), camp.ID)
		}(b)
	}
}

// runCampaign sends until the queue drains, the campaign leaves 'running', or a
// gate (working hours, daily cap, sender health) stops it.
func runCampaign(ctx context.Context, broadcastID string) {
	b, err := dbGetBroadcast(ctx, broadcastID)
	if err != nil {
		log.Printf("[broadcast] load %s: %v", broadcastID, err)
		return
	}
	thr := mergeThrottle(b.Throttle)
	loc := thr.location()

	var prov Provider = newWhatsmeowProvider()
	if b.DryRun {
		prov = &dryRunProvider{inner: prov}
	}

	// Media is read once per campaign, not per recipient.
	var media []byte
	var mediaName string
	if b.MediaPath != "" {
		path := filepath.Join(mediaDir(), filepath.Base(b.MediaPath))
		media, err = os.ReadFile(path)
		if err != nil {
			msg := fmt.Sprintf("gagal baca media: %v", err)
			log.Printf("[broadcast] %s: %s", broadcastID, msg)
			_ = dbSetBroadcastError(ctx, broadcastID, msg)
			_ = dbSetBroadcastStatus(ctx, broadcastID, BStatusPaused)
			return
		}
		mediaName = filepath.Base(b.MediaPath)
	}

	sentThisRun := 0

	for {
		// Re-read status every iteration so pause/cancel takes effect within
		// one message, without any goroutine-cancellation plumbing.
		status, err := dbBroadcastStatus(ctx, broadcastID)
		if err != nil || status != BStatusRunning {
			return
		}

		now := time.Now().In(loc)
		if !thr.withinWorkingWindow(now) {
			log.Printf("[broadcast] %s: di luar jam kerja, menunggu", broadcastID)
			_ = dbSetBroadcastError(ctx, broadcastID, "menunggu jam kerja")
			if !sleepInterruptible(ctx, broadcastID, 60*time.Second) {
				return
			}
			continue
		}

		// Daily cap is counted per sender across every campaign — a per-campaign
		// cap would not compose (three 150-message campaigns on one number).
		sentToday, err := dbSentTodayBySender(ctx, b.SenderID, loc)
		if err != nil {
			log.Printf("[broadcast] daily cap check %s: %v", broadcastID, err)
			return
		}
		if sentToday >= thr.DailyCap {
			msg := fmt.Sprintf("batas harian tercapai (%d/%d) — lanjut besok", sentToday, thr.DailyCap)
			log.Printf("[broadcast] %s: %s", broadcastID, msg)
			_ = dbSetBroadcastError(ctx, broadcastID, msg)
			if !sleepInterruptible(ctx, broadcastID, 5*time.Minute) {
				return
			}
			continue
		}

		if err := prov.Ready(ctx, b.SenderID); err != nil {
			msg := fmt.Sprintf("pengirim tidak siap: %v", err)
			log.Printf("[broadcast] %s: %s — campaign dipause", broadcastID, msg)
			_ = dbSetBroadcastError(ctx, broadcastID, msg)
			_ = dbSetBroadcastStatus(ctx, broadcastID, BStatusPaused)
			return
		}

		r, err := dbClaimNextRecipient(ctx, broadcastID)
		if err != nil {
			log.Printf("[broadcast] claim %s: %v", broadcastID, err)
			return
		}
		if r == nil {
			break // queue drained (or everything is waiting on a backoff)
		}

		// Guards re-checked at send time against fresh data.
		if reason := sendTimeSkipReason(ctx, b, thr, r.Phone); reason != "" {
			_ = dbMarkRecipientSkipped(ctx, r.ID, reason)
			continue
		}

		msg := OutboundMessage{
			Text:      renderTemplate(b.MessageText, r.Vars),
			Media:     media,
			MediaMime: b.MediaMime,
			MediaName: mediaName,
			Separate:  b.MediaMode == "separate",
		}

		sendCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		result, sendErr := prov.Send(sendCtx, b.SenderID, r.Phone, msg)
		cancel()

		if sendErr == nil {
			_ = dbMarkRecipientSent(ctx, r.ID, result.ProviderMsgID)
			sentThisRun++
		} else {
			se, ok := sendErr.(*SendError)
			retryable := ok && se.Retryable
			switch {
			case retryable && r.Attempts < thr.MaxAttempts:
				backoff := time.Duration(thr.RetryBackoffSec) * time.Second
				for i := 1; i < r.Attempts; i++ {
					backoff *= 2
				}
				if max := time.Duration(thr.RetryBackoffMaxSec) * time.Second; backoff > max {
					backoff = max
				}
				_ = dbMarkRecipientRetry(ctx, r.ID, sendErr.Error(), time.Now().Add(backoff))
			default:
				_ = dbMarkRecipientFailed(ctx, r.ID, sendErr.Error())
			}
			if ok && se.FatalForSender {
				msg := fmt.Sprintf("pengirim bermasalah: %v", sendErr)
				log.Printf("[broadcast] %s: %s — campaign dipause", broadcastID, msg)
				_ = dbSetBroadcastError(ctx, broadcastID, msg)
				_ = dbSetBroadcastStatus(ctx, broadcastID, BStatusPaused)
				return
			}
		}

		// Pace. Warm-up doubles the delay for the first N sends so a number
		// never goes from idle to full rate in one step.
		delay := jitterDelay(thr)
		if sentThisRun <= thr.WarmupFirstN {
			delay *= 2
		}
		if thr.BatchSize > 0 && sentThisRun > 0 && sentThisRun%thr.BatchSize == 0 {
			delay = time.Duration(thr.BatchPauseSec) * time.Second
			log.Printf("[broadcast] %s: jeda batch %s setelah %d pesan", broadcastID, delay, sentThisRun)
		}
		if !sleepInterruptible(ctx, broadcastID, delay) {
			return
		}
	}

	if remaining, err := dbPendingRecipientCount(ctx, broadcastID); err == nil && remaining == 0 {
		_ = dbFinishBroadcast(ctx, broadcastID)
		log.Printf("[broadcast] %s selesai (%d terkirim di sesi ini)", broadcastID, sentThisRun)
	}
}

// sendTimeSkipReason re-checks the exclusions against current data. The build
// pass already filtered these, but a campaign scheduled for next week must
// honour an opt-out recorded tomorrow.
func sendTimeSkipReason(ctx context.Context, b *Broadcast, thr Throttle, phone string) string {
	if !validWAPhone(phone) {
		return SkipInvalid
	}
	if out, err := dbIsOptedOut(ctx, phone); err == nil && out {
		return SkipOptOut
	}
	if !b.IncludeBlacklist {
		if set, err := dbBlacklistedPhoneSet(ctx, []string{phone}); err == nil && set[phone] {
			return SkipBlacklist
		}
	}
	if recent, err := dbRecentlyContacted(ctx, phone, thr.CooldownDays, b.ID); err == nil && recent {
		return SkipCooldown
	}
	return ""
}

func jitterDelay(thr Throttle) time.Duration {
	span := thr.MaxDelaySec - thr.MinDelaySec
	sec := thr.MinDelaySec
	if span > 0 {
		sec += rand.Intn(span + 1)
	}
	return time.Duration(sec) * time.Second
}

// sleepInterruptible waits, waking early if the campaign stops being 'running'.
// Returns false when the caller should stop.
func sleepInterruptible(ctx context.Context, broadcastID string, d time.Duration) bool {
	const step = 5 * time.Second
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining > step {
			remaining = step
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(remaining):
		}
		status, err := dbBroadcastStatus(ctx, broadcastID)
		if err != nil || status != BStatusRunning {
			return false
		}
	}
	return true
}

func mediaDir() string {
	return envOr("BROADCAST_MEDIA_DIR", "/data/broadcast-media")
}
