// Package mongo ports mongoService.js — syncs chat history and user data to
// MongoDB Atlas. All writes are non-blocking: the caller never waits for the
// network; at worst a message is dropped when the queue is full.
package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	batchSize             = 10
	batchInterval         = 30 * time.Second
	maxQueueSize          = 1000
	forceProcessThreshold = 50
)

// historyMessage mirrors the document shape from mongoService.saveChatHistory.
type historyMessage struct {
	PhoneNumber string    `bson:"phoneNumber"`
	ID          string    `bson:"id"`
	Timestamp   time.Time `bson:"timestamp"`
	Role        string    `bson:"role"`
	Content     string    `bson:"content"`
	Type        string    `bson:"type"`
	Processed   bool      `bson:"processed"`
	QueuedAt    time.Time `bson:"queuedAt"`
}

// Client holds the Atlas connection and the in-memory write queue.
type Client struct {
	mu          sync.Mutex
	client      *driver.Client
	db          *driver.Database
	connected   bool
	queue       []historyMessage
	isProcessing bool
	stopCh      chan struct{}
	collHistory string
	collUsers   string
}

// New creates a Client and starts the background batch flusher.
// Call Connect() before syncing.
func New(collHistory, collUsers string) *Client {
	if collHistory == "" {
		collHistory = "chat_history"
	}
	if collUsers == "" {
		collUsers = "users"
	}
	c := &Client{
		collHistory: collHistory,
		collUsers:   collUsers,
		stopCh:      make(chan struct{}),
	}
	go c.runBatchFlusher()
	return c
}

// Connect establishes the Atlas connection (mirrors mongoService.connect).
func (c *Client) Connect(ctx context.Context, uri, dbName string) error {
	if uri == "" {
		return fmt.Errorf("mongo: MONGODB_URI is empty")
	}
	opts := options.Client().ApplyURI(uri).
		SetMaxPoolSize(10).
		SetServerSelectionTimeout(10 * time.Second).
		SetConnectTimeout(10 * time.Second)

	mc, err := driver.Connect(opts)
	if err != nil {
		return fmt.Errorf("mongo: connect: %w", err)
	}
	if err := mc.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		_ = mc.Disconnect(ctx)
		return fmt.Errorf("mongo: ping failed: %w", err)
	}
	c.mu.Lock()
	c.client = mc
	c.db = mc.Database(dbName)
	c.connected = true
	c.mu.Unlock()
	log.Println("mongo: connected to MongoDB Atlas")
	return nil
}

// Disconnect closes the Atlas connection.
func (c *Client) Disconnect(ctx context.Context) {
	close(c.stopCh)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		_ = c.client.Disconnect(ctx)
		c.connected = false
	}
}

// SyncHistory queues a chat message for batch insert into the chat_history
// collection. Non-blocking. Mirrors mongoService.saveChatHistory().
func (c *Client) SyncHistory(phone, role, content string) {
	if phone == "" || role == "" || content == "" {
		return
	}
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return
	}
	if len(c.queue) >= maxQueueSize {
		c.mu.Unlock()
		log.Printf("mongo: queue full (%d), dropping history for %s", maxQueueSize, phone)
		return
	}
	now := time.Now()
	msg := historyMessage{
		PhoneNumber: phone,
		ID:          fmt.Sprintf("%d%s", now.UnixMilli(), randStr(9)),
		Timestamp:   now,
		Role:        role,
		Content:     content,
		Type:        "text",
		Processed:   false,
		QueuedAt:    now,
	}
	c.queue = append(c.queue, msg)
	shouldFlush := len(c.queue) >= forceProcessThreshold || len(c.queue) >= batchSize
	c.mu.Unlock()

	if shouldFlush {
		go func() {
			if err := c.processBatch(context.Background()); err != nil {
				log.Printf("mongo: background flush error: %v", err)
			}
		}()
	}
}

// SyncUserData upserts per-phone user fields into the users collection.
// Non-blocking. Mirrors mongoService.saveUserData / updateUserFields.
func (c *Client) SyncUserData(phone string, fields map[string]any) {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return
	}
	db := c.db
	coll := c.collUsers
	c.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		now := time.Now()
		setFields := bson.D{}
		for k, v := range fields {
			setFields = append(setFields, bson.E{Key: k, Value: v})
		}
		setFields = append(setFields, bson.E{Key: "updatedAt", Value: now})

		_, err := db.Collection(coll).UpdateOne(ctx,
			bson.D{{Key: "phoneNumber", Value: phone}},
			bson.D{
				{Key: "$set", Value: setFields},
				{Key: "$setOnInsert", Value: bson.D{
					{Key: "phoneNumber", Value: phone},
					{Key: "createdAt", Value: now},
				}},
			},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			log.Printf("mongo: SyncUserData %s: %v", phone, err)
		}
	}()
}

// GetUserData fetches a user document from MongoDB.
// Returns (nil, nil) when not found. Mirrors mongoService.getUserData().
func (c *Client) GetUserData(ctx context.Context, phone string) (map[string]json.RawMessage, error) {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil, nil
	}
	db := c.db
	coll := c.collUsers
	c.mu.Unlock()

	var raw bson.M
	err := db.Collection(coll).FindOne(ctx, bson.D{{Key: "phoneNumber", Value: phone}}).Decode(&raw)
	if err == driver.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mongo: GetUserData %s: %w", phone, err)
	}

	out := make(map[string]json.RawMessage)
	for k, v := range raw {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out, nil
}

// GetHistory fetches recent chat history for a phone. Mirrors mongoService.getChatHistory().
func (c *Client) GetHistory(ctx context.Context, phone string, limit int) ([]historyMessage, error) {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil, nil
	}
	db := c.db
	coll := c.collHistory
	c.mu.Unlock()

	type conv struct {
		Messages []historyMessage `bson:"messages"`
	}
	var doc conv
	err := db.Collection(coll).FindOne(ctx,
		bson.D{{Key: "phoneNumber", Value: phone}},
	).Decode(&doc)
	if err == driver.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mongo: GetHistory %s: %w", phone, err)
	}
	msgs := doc.Messages
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

// processBatch drains up to batchSize messages from the queue and bulk-writes
// them into the chat_history collection (upsert per phoneNumber with $push).
// Mirrors mongoService.processBatch().
func (c *Client) processBatch(ctx context.Context) error {
	c.mu.Lock()
	if c.isProcessing || len(c.queue) == 0 {
		c.mu.Unlock()
		return nil
	}
	c.isProcessing = true
	end := batchSize
	if end > len(c.queue) {
		end = len(c.queue)
	}
	batch := c.queue[:end]
	c.queue = c.queue[end:]
	db := c.db
	coll := c.collHistory
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.isProcessing = false
		c.mu.Unlock()
	}()

	// Group by phone to build bulkWrite operations
	byPhone := map[string][]historyMessage{}
	for _, m := range batch {
		byPhone[m.PhoneNumber] = append(byPhone[m.PhoneNumber], m)
	}

	models := make([]driver.WriteModel, 0, len(byPhone))
	now := time.Now()
	for phone, msgs := range byPhone {
		docs := make([]any, len(msgs))
		for i, m := range msgs {
			docs[i] = m
		}
		model := driver.NewUpdateOneModel().
			SetFilter(bson.D{{Key: "phoneNumber", Value: phone}}).
			SetUpdate(bson.D{
				{Key: "$push", Value: bson.D{
					{Key: "messages", Value: bson.D{
						{Key: "$each", Value: docs},
					}},
				}},
				{Key: "$set", Value: bson.D{
					{Key: "updatedAt", Value: now},
				}},
				{Key: "$setOnInsert", Value: bson.D{
					{Key: "phoneNumber", Value: phone},
					{Key: "createdAt", Value: now},
				}},
			}).
			SetUpsert(true)
		models = append(models, model)
	}

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := db.Collection(coll).BulkWrite(tctx, models); err != nil {
		return fmt.Errorf("mongo: BulkWrite chat_history: %w", err)
	}
	log.Printf("mongo: flushed %d messages in batch", len(batch))
	return nil
}

// runBatchFlusher ticks every batchInterval and drains the queue.
func (c *Client) runBatchFlusher() {
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if err := c.processBatch(context.Background()); err != nil {
				log.Printf("mongo: batch flush error: %v", err)
			}
		}
	}
}

func randStr(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
