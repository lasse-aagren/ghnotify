package tray

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/notify"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/boyvinall/ghnotify/internal/updater"
	"github.com/getlantern/systray"
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

func onReady(opts Options) func() {
	return func() {
		systray.SetIcon(iconBytes())
		systray.SetTooltip("ghnotify — GitHub PR monitor")

		// My PRs section — all slots created BEFORE the separator.
		myList := newPRList(opts.Config.MaxPRsPerSection, opts.Auth, opts.Snooze, "My Pull Requests", false)
		myList.build()

		systray.AddSeparator()

		// Review Requests section — all slots created BEFORE the separator.
		reviewList := newPRList(opts.Config.MaxPRsPerSection, opts.Auth, opts.Snooze, "Review Requests", true)
		reviewList.build()

		systray.AddSeparator()

		mPrefs := systray.AddMenuItem("Preferences…", "Open config file")
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
			myCount := myList.update(opts.Poll.MyPRs())
			revCount := reviewList.update(opts.Poll.ReviewRequests())
			if myCount+revCount > 0 {
				systray.SetIcon(iconActiveBytes())
			} else {
				systray.SetIcon(iconBytes())
			}
			opts.Notif.HandleChanges(changes)
		})

		go func() {
			for {
				select {
				case <-mPrefs.ClickedCh:
					openConfig()
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
	path, err := config.ConfigFilePath()
	if err != nil {
		return
	}
	_ = exec.Command("open", path).Start()
}
