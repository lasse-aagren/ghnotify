package tray

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/getlantern/systray"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/notify"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/boyvinall/ghnotify/internal/updater"
)

// Options bundles all dependencies for the tray.
type Options struct {
	Config  *config.AppConfig
	Auth    *auth.Manager
	Poll    *poller.Manager
	Snooze  *poller.SnoozeStore
	Notif   *notify.Notifier
	Updater *updater.Updater
}

// Run blocks until the user quits the app. Call from main().
func Run(opts Options) {
	systray.Run(onReady(opts), onExit)
}

func setIcon(active bool) {
	if active {
		systray.SetIcon(iconActiveBytes())
	} else {
		systray.SetIcon(iconBytes())
	}
}

func onReady(opts Options) func() {
	return func() {
		slog.Debug("setting up tray menu")

		setIcon(false)
		systray.SetTooltip("ghnotify — GitHub PR monitor")

		// My PRs section — all slots created BEFORE the separator.
		myList := newPRList(opts.Config.MaxPRsPerSection, opts.Auth, opts.Snooze, "My Pull Requests", false, opts.Poll.MyPRs)
		myList.build()

		systray.AddSeparator()

		// Review Requests section — all slots created BEFORE the separator.
		enableApprovePR := false
		reviewList := newPRList(opts.Config.MaxPRsPerSection, opts.Auth, opts.Snooze, "Review Requests", enableApprovePR, opts.Poll.ReviewRequests)
		reviewList.build()

		systray.AddSeparator()

		mPrefs := systray.AddMenuItem("Preferences…", "Open config file")
		mAckAll := systray.AddMenuItem("Acknowledge All", "Dismiss active icon until next change")
		mClearSnooze := systray.AddMenuItem("Clear Snoozed Items", "Unsnooze all snoozed PRs")
		mUpdate := systray.AddMenuItem("Check for updates", "")
		mQuit := systray.AddMenuItem("Quit", "Quit ghnotify")

		// Track latest update URL so the click handler knows what to do.
		var latestURL string

		// Start the background update checker.
		ctx, cancelUpdater := context.WithCancel(context.Background())
		opts.Updater.Start(ctx, func(tag, downloadURL string) {
			latestURL = downloadURL
			mUpdate.SetTitle(fmt.Sprintf("Update available: %s  →", tag))
		})

		// Subscribe to poll changes.
		opts.Poll.OnChange(func(changes []poller.Change) {
			slog.Debug("poll changes", "count", len(changes))

			myCount := myList.update()
			revCount := reviewList.update()
			active := myCount+revCount > 0
			setIcon(active)
			opts.Notif.HandleChanges(changes)
		})

		go func() {
			for {
				select {
				case <-mAckAll.ClickedCh:
					setIcon(false)
				case <-mPrefs.ClickedCh:
					openConfig()
				case <-mClearSnooze.ClickedCh:
					opts.Snooze.ClearAll()
					myList.update()
					reviewList.update()
				case <-mUpdate.ClickedCh:
					if latestURL != "" {
						_ = exec.Command("open", latestURL).Start()
					} else {
						opts.Updater.CheckNow()
					}
				case <-mQuit.ClickedCh:
					cancelUpdater()
					systray.Quit()
					return
				}
			}
		}()
	}
}

func onExit() {}

func openConfig() {
	path, err := config.GetFilePath()
	if err != nil {
		return
	}
	_ = exec.Command("open", path).Start()
}
