package tray

import (
	"fmt"
	"sort"
	"sync"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/github"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/getlantern/systray"
)

// prList manages one PR section (My PRs or Review Requests) in the menubar.
// Slots are pre-created at build time; Show/Hide is used to manage visibility.
type prList struct {
	mu       sync.Mutex
	header   *systray.MenuItem // "My Pull Requests (N)" — disabled
	slots    []*prItem         // pre-created pool, one per max-allowed PR
	mMore    *systray.MenuItem // "… and N more" (shown when truncated)
	maxItems int
	label    string // "My Pull Requests" or "Review Requests"
	mgr      *auth.Manager
	snooze   *poller.SnoozeStore
}

func newPRList(maxItems int, mgr *auth.Manager, snooze *poller.SnoozeStore, label string) *prList {
	return &prList{maxItems: maxItems, label: label, mgr: mgr, snooze: snooze}
}

// build creates the header and all pre-allocated slot items in the menu.
// Must be called from the systray onReady goroutine, in the correct menu order.
func (l *prList) build() {
	l.header = systray.AddMenuItem(l.label, "")
	l.header.Disable()

	l.slots = make([]*prItem, l.maxItems)
	for i := range l.slots {
		l.slots[i] = newPRItem(l.mgr, l.snooze)
	}

	l.mMore = systray.AddMenuItem("", "")
	l.mMore.Disable()
	l.mMore.Hide()
}

// update re-renders the section with the provided PR list (full server snapshot).
// It filters snoozed PRs, sorts, caps at maxItems, and returns the visible count.
func (l *prList) update(prs []github.PR) int {
	// Filter snoozed.
	visible := prs[:0:0] // reuse backing array header without aliasing
	visible = make([]github.PR, 0, len(prs))
	for _, pr := range prs {
		if !l.snooze.IsSnoozed(pr.Key(), pr.UpdatedAt) {
			visible = append(visible, pr)
		}
	}

	// Sort: server → repo → number.
	sort.Slice(visible, func(i, j int) bool {
		a, b := visible[i], visible[j]
		if a.Server != b.Server {
			return a.Server < b.Server
		}
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		return a.Number < b.Number
	})

	total := len(visible)
	display := visible
	overflow := 0
	if total > l.maxItems {
		display = visible[:l.maxItems]
		overflow = total - l.maxItems
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Assign PRs to slots.
	for i, slot := range l.slots {
		if i < len(display) {
			slot.assign(display[i])
		} else {
			slot.clear()
		}
	}

	// "… and N more" overflow indicator.
	if overflow > 0 {
		l.mMore.SetTitle(fmt.Sprintf("… and %d more (open GitHub to see)", overflow))
		l.mMore.Show()
	} else {
		l.mMore.Hide()
	}

	// Update section header with count.
	if total == 0 {
		l.header.SetTitle(l.label)
	} else {
		l.header.SetTitle(fmt.Sprintf("%s  (%d)", l.label, total))
	}

	return total
}
