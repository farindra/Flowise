// Package state provides per-phone-number persistence backed by sqlite,
// porting database.js's getUserData/setUserData (this.data.memori[phone])
// and addToHistory. An optional MongoSyncer can be injected to mirror writes
// to MongoDB Atlas (Phase 4).
package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// MongoSyncer is the subset of mongo.Client methods used by Store.
// Defined as an interface so Store doesn't import the mongo package directly.
type MongoSyncer interface {
	SyncHistory(phone, role, content string)
	SyncUserData(phone string, fields map[string]any)
}

// StateIdle mirrors messageHandler's this.states.IDLE.
const StateIdle = "idle"

// Store is a per-phone-number key/value store backed by sqlite (DATA_DIR/state.db,
// separate from whatsmeow's own session database).
type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	mongo MongoSyncer // optional; nil = no cloud sync
}

// SetMongoSyncer injects a MongoDB syncer after the store is opened.
func (s *Store) SetMongoSyncer(m MongoSyncer) {
	s.mu.Lock()
	s.mongo = m
	s.mu.Unlock()
}

// Open creates (if needed) DATA_DIR/state.db and its schema.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "state.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}

	const schema = `
CREATE TABLE IF NOT EXISTS user_data (
	phone_number TEXT PRIMARY KEY,
	data TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	phone_number TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	timestamp INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_history_phone ON chat_history(phone_number);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate state db: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying sqlite database.
func (s *Store) Close() error {
	return s.db.Close()
}

// load returns the stored data object for phone, or an empty map if the
// phone has no row yet.
func (s *Store) load(phone string) (map[string]json.RawMessage, error) {
	var raw string
	err := s.db.QueryRow(`SELECT data FROM user_data WHERE phone_number = ?`, phone).Scan(&raw)
	if err == sql.ErrNoRows {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}

	data := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Store) save(phone string, data map[string]json.RawMessage) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO user_data (phone_number, data, updated_at) VALUES (?, ?, ?)
ON CONFLICT(phone_number) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at
`, phone, string(raw), time.Now().UnixMilli())
	return err
}

// Get ports database.js's getUserData(phone, key): looks up a single key
// within the per-phone data object. Returns found=false if the phone has no
// stored data, or the key is absent.
func (s *Store) Get(phone, key string, out any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.load(phone)
	if err != nil {
		return false, err
	}

	raw, ok := data[key]
	if !ok {
		return false, nil
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return false, err
		}
	}
	return true, nil
}

// GetAll ports database.js's getUserData(phone) (key omitted): returns the
// full per-phone data object.
func (s *Store) GetAll(phone string) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(phone)
}

// Set ports database.js's setUserData(phone, key, value): sets a single key
// within the per-phone data object and refreshes lastActivity, mirroring
// `memori[phone][key] = value; memori[phone].lastActivity = Date.now()`.
func (s *Store) Set(phone, key string, val any) error {
	s.mu.Lock()

	data, err := s.load(phone)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	raw, err := json.Marshal(val)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	data[key] = raw

	now := time.Now().UnixMilli()
	lastActivity, err := json.Marshal(now)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	data["lastActivity"] = lastActivity

	if err := s.save(phone, data); err != nil {
		s.mu.Unlock()
		return err
	}

	m := s.mongo
	s.mu.Unlock()

	if m != nil {
		m.SyncUserData(phone, map[string]any{
			key:          val,
			"lastActivity": now,
		})
	}
	return nil
}

// userState is the shape stored under the "state" key by SetUserState.
type userState struct {
	State     string `json:"state"`
	Timestamp int64  `json:"timestamp"`
}

// GetUserState ports messageHandler.getUserState (line 2562): returns the
// stored state, or StateIdle if none is set.
func (s *Store) GetUserState(phone string) (string, error) {
	var st userState
	found, err := s.Get(phone, "state", &st)
	if err != nil {
		return "", err
	}
	if found && st.State != "" {
		return st.State, nil
	}
	return StateIdle, nil
}

// SetUserState ports messageHandler.setUserState (line 2570).
func (s *Store) SetUserState(phone, newState string) error {
	return s.Set(phone, "state", userState{State: newState, Timestamp: time.Now().UnixMilli()})
}

// ClearUserData ports database.js's clearUserData (line 289): resets the
// stored data for phone to just {lastActivity}, discarding all other keys.
func (s *Store) ClearUserData(phone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := map[string]json.RawMessage{}
	lastActivity, err := json.Marshal(time.Now().UnixMilli())
	if err != nil {
		return err
	}
	data["lastActivity"] = lastActivity
	return s.save(phone, data)
}

// AddToHistory ports database.js's addToHistory: appends one message to the
// local sqlite chat_history table, then asynchronously queues it for MongoDB.
func (s *Store) AddToHistory(phone, role, content string) error {
	_, err := s.db.Exec(`INSERT INTO chat_history (phone_number, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		phone, role, content, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	s.mu.Lock()
	m := s.mongo
	s.mu.Unlock()
	if m != nil {
		m.SyncHistory(phone, role, content)
	}
	return nil
}
