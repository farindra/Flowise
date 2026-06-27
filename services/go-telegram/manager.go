package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// BotManager holds all active bots and routes webhook calls by bot ID.
type BotManager struct {
	mu           sync.RWMutex
	bots         map[string]*Bot // key: bot UUID
	flowiseBase  string          // e.g. https://agentic.oceanbearings.co.id
	flowiseKey   string
	webhookBase  string // e.g. https://agentic.oceanbearings.co.id/telegram
	timeout      time.Duration
	waitInterval time.Duration
}

func NewBotManager(flowiseBase, flowiseKey, webhookBase string, timeout, waitInterval time.Duration) *BotManager {
	return &BotManager{
		bots:         make(map[string]*Bot),
		flowiseBase:  flowiseBase,
		flowiseKey:   flowiseKey,
		webhookBase:  webhookBase,
		timeout:      timeout,
		waitInterval: waitInterval,
	}
}

// LoadAll loads all active bots from DB and registers webhooks.
func (m *BotManager) LoadAll(ctx context.Context) error {
	records, err := dbListBots(ctx)
	if err != nil {
		return fmt.Errorf("list bots: %w", err)
	}
	for _, r := range records {
		if !r.Active {
			continue
		}
		bot := m.recordToBot(r)
		m.mu.Lock()
		m.bots[r.ID] = bot
		m.mu.Unlock()
		if m.webhookBase != "" {
			go bot.registerWebhook(m.webhookBase + "/webhook/" + r.ID)
		}
	}
	log.Printf("[manager] loaded %d active bots", len(records))
	return nil
}

// Add inserts a bot into DB, starts it, and registers its webhook.
func (m *BotManager) Add(ctx context.Context, r *BotRecord) (string, error) {
	id, err := dbCreateBot(ctx, r)
	if err != nil {
		return "", err
	}
	r.ID = id
	bot := m.recordToBot(*r)
	m.mu.Lock()
	m.bots[id] = bot
	m.mu.Unlock()
	if m.webhookBase != "" {
		go bot.registerWebhook(m.webhookBase + "/webhook/" + id)
	}
	return id, nil
}

// Update modifies bot config in DB and restarts the in-memory bot.
func (m *BotManager) Update(ctx context.Context, r *BotRecord) error {
	if err := dbUpdateBot(ctx, r); err != nil {
		return err
	}
	full, err := dbGetBot(ctx, r.ID)
	if err != nil {
		return err
	}
	bot := m.recordToBot(*full)
	m.mu.Lock()
	m.bots[r.ID] = bot
	m.mu.Unlock()
	if m.webhookBase != "" && full.Active {
		go bot.registerWebhook(m.webhookBase + "/webhook/" + r.ID)
	}
	return nil
}

// Remove deletes the bot from DB and stops it.
func (m *BotManager) Remove(ctx context.Context, id string) error {
	if err := dbDeleteBot(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.bots, id)
	m.mu.Unlock()
	return nil
}

// Handler returns an http.HandlerFunc that routes to the right bot by URL path.
// Path must contain the bot ID as the last segment: /webhook/<id>
func (m *BotManager) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract bot ID from path: /webhook/<id>
		id := r.PathValue("id")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		m.mu.RLock()
		bot, ok := m.bots[id]
		m.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		bot.handler(w, r)
	}
}

func (m *BotManager) recordToBot(r BotRecord) *Bot {
	flowiseURL := m.flowiseBase + "/api/v1/prediction/" + r.ChatflowID
	return newBot(
		r.Name,
		r.Token,
		flowiseURL,
		m.flowiseKey,
		"",
		r.HumanContact,
		parseIDSet(r.AllowUserIDs),
		r.DisableUpload,
		m.timeout,
		m.waitInterval,
	)
}
