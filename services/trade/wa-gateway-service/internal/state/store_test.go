package state

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore_GetSetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	var got string
	found, err := s.Get(phone, "company", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Errorf("Get() found = true for unset key, want false")
	}

	if err := s.Set(phone, "company", "Perorangan"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	found, err = s.Get(phone, "company", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || got != "Perorangan" {
		t.Errorf("Get() = (%v, %q), want (true, Perorangan)", found, got)
	}
}

func TestStore_SetUpdatesLastActivity(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	if err := s.Set(phone, "company", "Perorangan"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	var lastActivity int64
	found, err := s.Get(phone, "lastActivity", &lastActivity)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || lastActivity == 0 {
		t.Errorf("lastActivity = (%v, %d), want (true, nonzero)", found, lastActivity)
	}
}

func TestStore_GetAll(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	if err := s.Set(phone, "company", "Perorangan"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := s.Set(phone, "region", "jakarta"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	all, err := s.GetAll(phone)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if string(all["company"]) != `"Perorangan"` {
		t.Errorf("all[company] = %s, want \"Perorangan\"", all["company"])
	}
	if string(all["region"]) != `"jakarta"` {
		t.Errorf("all[region] = %s, want \"jakarta\"", all["region"])
	}
	if _, ok := all["lastActivity"]; !ok {
		t.Error("all[lastActivity] missing")
	}
}

func TestStore_SetNilClearsValue(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	if err := s.Set(phone, "customerNotFound", true); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := s.Set(phone, "customerNotFound", nil); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	all, err := s.GetAll(phone)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if string(all["customerNotFound"]) != "null" {
		t.Errorf("all[customerNotFound] = %s, want null", all["customerNotFound"])
	}
}

func TestStore_UserState(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	got, err := s.GetUserState(phone)
	if err != nil {
		t.Fatalf("GetUserState() error = %v", err)
	}
	if got != StateIdle {
		t.Errorf("GetUserState() = %q, want %q (default)", got, StateIdle)
	}

	if err := s.SetUserState(phone, "ASK_REGION_CHECKOUT"); err != nil {
		t.Fatalf("SetUserState() error = %v", err)
	}

	got, err = s.GetUserState(phone)
	if err != nil {
		t.Fatalf("GetUserState() error = %v", err)
	}
	if got != "ASK_REGION_CHECKOUT" {
		t.Errorf("GetUserState() = %q, want ASK_REGION_CHECKOUT", got)
	}
}

func TestStore_AddToHistory(t *testing.T) {
	s := newTestStore(t)
	phone := "6281234567890"

	if err := s.AddToHistory(phone, "user", "halo"); err != nil {
		t.Fatalf("AddToHistory() error = %v", err)
	}
	if err := s.AddToHistory(phone, "assistant", "halo juga"); err != nil {
		t.Fatalf("AddToHistory() error = %v", err)
	}

	rows, err := s.db.Query(`SELECT role, content FROM chat_history WHERE phone_number = ? ORDER BY id`, phone)
	if err != nil {
		t.Fatalf("query chat_history: %v", err)
	}
	defer rows.Close()

	var got []struct{ role, content string }
	for rows.Next() {
		var r, c string
		if err := rows.Scan(&r, &c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, struct{ role, content string }{r, c})
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].role != "user" || got[0].content != "halo" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].role != "assistant" || got[1].content != "halo juga" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestStore_OpenCreatesDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer s.Close()
}
