package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type SessionManager struct {
	mu             sync.RWMutex
	sessions       map[string]*WASession
	flowiseBaseURL string
	flowiseAPIKey  string
	dataDir        string
	timeout        time.Duration
}

func NewSessionManager(flowiseBaseURL, flowiseAPIKey, dataDir string, timeout time.Duration) *SessionManager {
	return &SessionManager{
		sessions:       map[string]*WASession{},
		flowiseBaseURL: flowiseBaseURL,
		flowiseAPIKey:  flowiseAPIKey,
		dataDir:        dataDir,
		timeout:        timeout,
	}
}

func (m *SessionManager) LoadAll(ctx context.Context) error {
	records, err := dbListSessions(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		if !r.Active {
			continue
		}
		s, err := newWASession(&r, m.flowiseBaseURL, m.flowiseAPIKey, m.dataDir, m.timeout)
		if err != nil {
			fmt.Printf("[manager] load %s error: %v\n", r.Name, err)
			continue
		}
		m.mu.Lock()
		m.sessions[r.ID] = s
		m.mu.Unlock()
		s.Connect(ctx)
		fmt.Printf("[manager] loaded session: %s (%s)\n", r.Name, r.ID)
	}
	return nil
}

// Add creates a new session in DB and starts it.
func (m *SessionManager) Add(ctx context.Context, r *SessionRecord) (string, error) {
	id, err := dbCreateSession(ctx, r)
	if err != nil {
		return "", err
	}
	r.ID = id

	s, err := newWASession(r, m.flowiseBaseURL, m.flowiseAPIKey, m.dataDir, m.timeout)
	if err != nil {
		_ = dbDeleteSession(ctx, id)
		return "", err
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	s.Connect(ctx)
	return id, nil
}

// Get returns a session by ID.
func (m *SessionManager) Get(id string) *WASession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Update updates metadata (name, chatflowID, etc.) in DB. Does not reconnect.
func (m *SessionManager) Update(ctx context.Context, r *SessionRecord) error {
	if err := dbUpdateSession(ctx, r); err != nil {
		return err
	}
	m.mu.Lock()
	if s, ok := m.sessions[r.ID]; ok {
		s.name = r.Name
		s.chatflowID = r.ChatflowID
		s.humanContact = r.HumanContact
		s.allowPhones = parsePhoneSet(r.AllowPhones)
		s.disableUpload = r.DisableUpload
	}
	m.mu.Unlock()
	return nil
}

// Remove logs out, stops, and deletes a session.
func (m *SessionManager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()

	if s != nil {
		_ = s.Logout(ctx)
	}
	return dbDeleteSession(ctx, id)
}

// Connect starts the QR/connect flow for an existing session.
func (m *SessionManager) Connect(ctx context.Context, id string) error {
	s := m.Get(id)
	if s == nil {
		return fmt.Errorf("session not found")
	}
	s.Connect(ctx)
	return nil
}

// Logout disconnects a specific session without deleting it.
func (m *SessionManager) Logout(ctx context.Context, id string) error {
	s := m.Get(id)
	if s == nil {
		return fmt.Errorf("session not found")
	}
	return s.Logout(ctx)
}

// ListStatus returns status info for all sessions.
func (m *SessionManager) ListStatus() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]map[string]any, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.StatusInfo())
	}
	return out
}
