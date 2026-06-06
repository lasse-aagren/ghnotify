package poller

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/boyvinall/ghnotify/internal/github"
)

const (
	SnoozeModeUntilChange = "until_change"
	SnoozeModeUntilTime   = "until_time"
)

// SnoozeEntry records the snooze state for one PR.
type SnoozeEntry struct {
	Mode      string    `json:"mode"`
	ExpiresAt time.Time `json:"expires_at,omitempty"` // for until_time

	// for until_change: snapshot of PR state at snooze time
	SnapshotRef          string             `json:"snapshot_ref,omitempty"`
	SnapshotCIStatus     github.CIStatus    `json:"snapshot_ci_status,omitempty"`
	SnapshotReviewState  github.ReviewState `json:"snapshot_review_state,omitempty"`
	SnapshotIsDraft      bool               `json:"snapshot_is_draft,omitempty"`
	SnapshotUpdated      time.Time          `json:"snapshot_updated_at,omitempty"` // fallback for old entries without SnapshotRef
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

// IsSnoozed reports whether pr is currently snoozed.
// For until_change snoozes, it lifts the snooze if the head SHA, CI status,
// review state, or draft status has changed since the snooze was set.
func (s *SnoozeStore) IsSnoozed(pr github.PR) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[pr.Key()]
	if !ok {
		return false
	}
	switch e.Mode {
	case SnoozeModeUntilTime:
		if time.Now().After(e.ExpiresAt) {
			delete(s.entries, pr.Key())
			s.save()
			return false
		}
		return true
	case SnoozeModeUntilChange:
		changed := s.hasChanged(e, pr)
		if changed {
			delete(s.entries, pr.Key())
			s.save()
			return false
		}
		return true
	}
	return false
}

func (s *SnoozeStore) hasChanged(e SnoozeEntry, pr github.PR) bool {
	if e.SnapshotRef != "" {
		// New-style entry: compare all tracked fields.
		return pr.HeadSHA != e.SnapshotRef ||
			pr.CIStatus != e.SnapshotCIStatus ||
			pr.ReviewState != e.SnapshotReviewState ||
			pr.IsDraft != e.SnapshotIsDraft
	}
	// Old-style entry: fall back to UpdatedAt comparison.
	return pr.UpdatedAt.After(e.SnapshotUpdated)
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
