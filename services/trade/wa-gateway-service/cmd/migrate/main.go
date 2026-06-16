// cmd/migrate pulls user data from MongoDB (Node bot's users collection) and
// pre-populates wa-gateway-service's sqlite state.db so existing users don't
// lose their company/region/isRegistered state after cutover.
//
// Usage (run ONCE, before or right after stopping the Node bot):
//
//	MONGODB_URI="mongodb+srv://..." \
//	DATA_DIR=/opt/oceanbearings/ob-bot/data/wa-gateway \
//	go run ./cmd/migrate
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"wa-gateway-service/internal/state"
)

func main() {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI is required")
	}
	dbName := os.Getenv("MONGODB_DB_NAME")
	if dbName == "" {
		dbName = "ocean_bearings"
	}
	collName := os.Getenv("MONGODB_COLLECTION_USERS")
	if collName == "" {
		collName = "users"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Connect to MongoDB.
	mc, err := driver.Connect(options.Client().ApplyURI(mongoURI).
		SetServerSelectionTimeout(15 * time.Second).
		SetConnectTimeout(15 * time.Second))
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer mc.Disconnect(context.Background()) //nolint:errcheck

	if err := mc.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		log.Fatalf("mongo ping: %v", err)
	}
	log.Printf("Connected to MongoDB %s/%s", dbName, collName)

	// Open sqlite store.
	store, err := state.Open(dataDir)
	if err != nil {
		log.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	// Pull all users.
	coll := mc.Database(dbName).Collection(collName)
	cursor, err := coll.Find(ctx, bson.D{})
	if err != nil {
		log.Fatalf("mongo find: %v", err)
	}
	defer cursor.Close(ctx) //nolint:errcheck

	migrated, skipped, failed := 0, 0, 0
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("decode error: %v", err)
			failed++
			continue
		}

		phone, ok := doc["phoneNumber"].(string)
		if !ok || phone == "" {
			skipped++
			continue
		}

		// Fields to migrate — only write keys that have non-empty values.
		fieldMap := map[string]string{
			"company":        "company",
			"region":         "region",
			"isRegistered":   "isRegistered",
			"lastGreet":      "lastGreet",
		}
		wrote := 0
		for mongoKey, stateKey := range fieldMap {
			val, exists := doc[mongoKey]
			if !exists || val == nil {
				continue
			}
			// Check existing sqlite value — don't overwrite if already set.
			var existing json.RawMessage
			found, _ := store.Get(phone, stateKey, &existing)
			if found && len(existing) > 2 { // non-null, non-empty
				continue
			}
			if err := store.Set(phone, stateKey, val); err != nil {
				log.Printf("set %s/%s: %v", phone, stateKey, err)
				failed++
				continue
			}
			wrote++
		}
		if wrote > 0 {
			migrated++
			log.Printf("migrated %s (%d keys)", phone, wrote)
		} else {
			skipped++
		}
	}
	if err := cursor.Err(); err != nil {
		log.Fatalf("cursor error: %v", err)
	}

	log.Printf("Done — migrated=%d skipped=%d failed=%d", migrated, skipped, failed)
}
