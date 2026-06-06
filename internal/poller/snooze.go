package poller

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	SnoozeModeUntilChange = "until_change"
	SnoozeModeUntilTime   = "until_time"
)

// SnoozeEntry records the snooze state for one PR.
type SnoozeEntry struct {
	Mode            string    `json:"mode"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`          // for until_time
	SnapshotUpdated time.Time `json:"snapshot_updated_at,omitempty"` // for until_change
}

// SnoozeStore persists per-PR snooze decisions to a JSON file.
type SnoozeStore struct {
	mu      sync.Mutex
	entries map[string]SnoozeEntry
	path    string
}

// NewSnoozeStore loads existing snooze state from path (creates empty store if
// the file doesn't exist yet).
func NewSnoozeStore(path string) *SnoozeStore {
	s := &SnoozeStore{entries: make(map[string]SnoozeEntry), path: path}
	s.load()
	return s
}

// Snooze registers a snooze entry for the given PR key.
func (s *SnoozeStore) Snooze(key string, entry SnoozeEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = entry
	s.save()
}

// Unsnooze clears any snooze entry for key.
func (s *SnoozeStore) Unsnooze(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	s.save()
}

// ClearAll removes all snooze entries.
func (s *SnoozeStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]SnoozeEntry)
	s.save()
}

// IsSnoozed reports whether the PR with key is currently snoozed.
// updatedAt should be PR.UpdatedAt; it is used to lift "until_change" snoozes.
func (s *SnoozeStore) IsSnoozed(key string, updatedAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok {
		return false
	}
	switch e.Mode {
	case SnoozeModeUntilTime:
		if time.Now().After(e.ExpiresAt) {
			delete(s.entries, key)
			s.save()
			return false
		}
		return true
	case SnoozeModeUntilChange:
		if updatedAt.After(e.SnapshotUpdated) {
			delete(s.entries, key)
			s.save()
			return false
		}
		return true
	}
	return false
}

// --- persistence -------------------------------------------------------------

type snoozeFile struct {
	Snoozed map[string]SnoozeEntry `json:"snoozed"`
}

func (s *SnoozeStore) save() {
	if s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(snoozeFile{Snoozed: s.entries}, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		log.Printf("ghnotify: save snooze: %v", err)
	}
}

func (s *SnoozeStore) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var f snoozeFile
	if err := json.Unmarshal(data, &f); err != nil {
		log.Printf("ghnotify: load snooze: %v", err)
		return
	}
	if f.Snoozed != nil {
		s.entries = f.Snoozed
	}
}
