package tray

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/github"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/getlantern/systray"
)

// prItem is one slot in a prList. Slots are pre-created and shown/hidden as the
// PR list changes. Each slot owns its full sub-menu (Open, Copy, Approve, Snooze…).
type prItem struct {
	mu sync.RWMutex
	pr *github.PR // nil = slot is unused

	// menu items (created once, never destroyed)
	mItem         *systray.MenuItem
	mOpen         *systray.MenuItem
	mCopy         *systray.MenuItem
	mApprove      *systray.MenuItem
	mSnooze       *systray.MenuItem // "Snooze…" parent
	mSnoozeChange *systray.MenuItem //   └── Until next change
	mSnooze1h     *systray.MenuItem //   └── 1 hour
	mSnooze8h     *systray.MenuItem //   └── 8 hours
	mSnooze24h    *systray.MenuItem //   └── 24 hours

	mgr    *auth.Manager
	snooze *poller.SnoozeStore
}

func newPRItem(mgr *auth.Manager, snooze *poller.SnoozeStore) *prItem {
	it := &prItem{mgr: mgr, snooze: snooze}
	it.mItem = systray.AddMenuItem("", "")
	it.mOpen = it.mItem.AddSubMenuItem("Open in Browser", "")
	it.mCopy = it.mItem.AddSubMenuItem("Copy URL", "")
	it.mApprove = it.mItem.AddSubMenuItem("Approve", "")
	it.mSnooze = it.mItem.AddSubMenuItem("Snooze…", "")
	it.mSnoozeChange = it.mSnooze.AddSubMenuItem("Until next change", "")
	it.mSnooze1h = it.mSnooze.AddSubMenuItem("1 hour", "")
	it.mSnooze8h = it.mSnooze.AddSubMenuItem("8 hours", "")
	it.mSnooze24h = it.mSnooze.AddSubMenuItem("24 hours", "")
	it.mItem.Hide()
	go it.listen()
	return it
}

// assign binds a PR to this slot and makes it visible.
func (it *prItem) assign(pr github.PR) {
	it.mu.Lock()
	it.pr = &pr
	it.mu.Unlock()

	it.mItem.SetTitle(formatPRTitle(pr))
	it.mItem.SetTooltip(fmt.Sprintf("%s › %s/%s #%d", pr.Server, pr.Owner, pr.Repo, pr.Number))
	it.mApprove.SetTitle("Approve")
	it.mApprove.Enable()
	it.mItem.Show()
}

// clear hides this slot and detaches the PR.
func (it *prItem) clear() {
	it.mu.Lock()
	it.pr = nil
	it.mu.Unlock()
	it.mItem.Hide()
}

func (it *prItem) currentPR() *github.PR {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return it.pr
}

func (it *prItem) listen() {
	for {
		select {
		case <-it.mOpen.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				_ = exec.Command("open", pr.URL).Start()
			}
		case <-it.mCopy.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				copyToClipboard(pr.URL)
			}
		case <-it.mApprove.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				go it.doApprove(pr)
			}
		case <-it.mSnoozeChange.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				it.snooze.Snooze(pr.Key(), poller.SnoozeEntry{
					Mode:            poller.SnoozeModeUntilChange,
					SnapshotUpdated: pr.UpdatedAt,
				})
			}
		case <-it.mSnooze1h.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				it.snooze.Snooze(pr.Key(), poller.SnoozeEntry{
					Mode:      poller.SnoozeModeUntilTime,
					ExpiresAt: time.Now().Add(time.Hour),
				})
			}
		case <-it.mSnooze8h.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				it.snooze.Snooze(pr.Key(), poller.SnoozeEntry{
					Mode:      poller.SnoozeModeUntilTime,
					ExpiresAt: time.Now().Add(8 * time.Hour),
				})
			}
		case <-it.mSnooze24h.ClickedCh:
			if pr := it.currentPR(); pr != nil {
				it.snooze.Snooze(pr.Key(), poller.SnoozeEntry{
					Mode:      poller.SnoozeModeUntilTime,
					ExpiresAt: time.Now().Add(24 * time.Hour),
				})
			}
		}
	}
}

func (it *prItem) doApprove(pr *github.PR) {
	token, err := it.mgr.GetToken(pr.Server)
	if err != nil || token == "" {
		log.Printf("ghnotify: approve: no token for %s", pr.Server)
		return
	}
	client := github.NewClient(pr.Server, token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Approve(ctx, pr.Owner, pr.Repo, pr.Number); err != nil {
		log.Printf("ghnotify: approve %s/%s#%d: %v", pr.Owner, pr.Repo, pr.Number, err)
		return
	}
	it.mApprove.SetTitle("Approved ✓")
	it.mApprove.Disable()
}

// --- formatting --------------------------------------------------------------

func formatPRTitle(pr github.PR) string {
	ci := ciGlyph(pr.CIStatus)
	rev := reviewGlyph(pr.ReviewState, pr.IsDraft)
	title := pr.Title
	if len(title) > 50 {
		title = title[:47] + "…"
	}
	return fmt.Sprintf("[%s][%s]  %s › %s  #%d  %s", ci, rev, pr.Server, pr.Repo, pr.Number, title)
}

func ciGlyph(s github.CIStatus) string {
	switch s {
	case github.CIPassing:
		return "✓"
	case github.CIFailing:
		return "✗"
	case github.CIPending:
		return "○"
	default:
		return "·"
	}
}

func reviewGlyph(s github.ReviewState, draft bool) string {
	if draft {
		return "~"
	}
	switch s {
	case github.ReviewApproved:
		return "✓"
	case github.ReviewChangesRequested:
		return "✗"
	default:
		return "○"
	}
}

// copyToClipboard writes text to the macOS clipboard via pbcopy.
func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}
