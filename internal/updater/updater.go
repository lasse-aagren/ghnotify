// Package updater checks GitHub Releases for a newer version of ghnotify and
// notifies the caller via a callback.
package updater

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	releasesAPI   = "https://api.github.com/repos/boyvinall/ghnotify/releases/latest"
	checkInterval = 24 * time.Hour
)

// Updater polls GitHub Releases for a newer version.
type Updater struct {
	currentVersion string

	mu      sync.Mutex
	onFound func(tag, downloadURL string) // set by Start, used by CheckNow
}

// NewUpdater creates an Updater for the running version.
// version should match the goreleaser-injected ldflags value (e.g. "1.2.3").
// If version is "dev" or empty, all checks are skipped.
func NewUpdater(version string) *Updater {
	return &Updater{currentVersion: version}
}

// Start begins the background check loop. onFound is called (in a goroutine)
// when a newer release is found, with the tag (e.g. "v1.2.3") and the release
// page URL. Start returns immediately.
func (u *Updater) Start(ctx context.Context, onFound func(tag, downloadURL string)) {
	u.mu.Lock()
	u.onFound = onFound
	u.mu.Unlock()

	go u.loop(ctx)
}

// CheckNow triggers an immediate out-of-cycle check.
func (u *Updater) CheckNow() {
	u.mu.Lock()
	fn := u.onFound
	u.mu.Unlock()
	if fn != nil {
		go u.check(fn)
	}
}

// --- internal ----------------------------------------------------------------

func (u *Updater) loop(ctx context.Context) {
	u.mu.Lock()
	fn := u.onFound
	u.mu.Unlock()

	// Check immediately on startup.
	u.check(fn)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.mu.Lock()
			fn = u.onFound
			u.mu.Unlock()
			u.check(fn)
		}
	}
}

type releaseResponse struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (u *Updater) check(onFound func(string, string)) {
	if u.currentVersion == "dev" || u.currentVersion == "" || onFound == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("ghnotify: update check: %v", err)
		return
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return
	}

	var rel releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return
	}
	if rel.Draft || rel.Prerelease || rel.TagName == "" {
		return
	}

	if isNewer(u.currentVersion, rel.TagName) {
		onFound(rel.TagName, rel.HTMLURL)
	}
}

// isNewer returns true when latest differs from current (after stripping "v"
// prefix). GitHub's /releases/latest always points to the most recent
// non-prerelease, so "different" reliably means "newer".
func isNewer(current, latest string) bool {
	c := strings.TrimPrefix(current, "v")
	l := strings.TrimPrefix(latest, "v")
	return c != "" && l != "" && c != l
}
