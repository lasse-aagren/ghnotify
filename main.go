package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/boyvinall/ghnotify/internal/auth"
	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/notify"
	"github.com/boyvinall/ghnotify/internal/poller"
	"github.com/boyvinall/ghnotify/internal/tray"
	"github.com/boyvinall/ghnotify/internal/updater"
	"github.com/urfave/cli/v3"
)

var version = "dev"

func main() {
	app := &cli.Command{
		Name:    "ghnotify",
		Usage:   "GitHub PR monitor in your menubar",
		Version: version,
		Action:  run,
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	snoozePath, err := config.SnoozeFilePath()
	if err != nil {
		return fmt.Errorf("snooze path: %w", err)
	}

	mgr := auth.NewManager(cfg)
	snooze := poller.NewSnoozeStore(snoozePath)
	poll := poller.NewManager(mgr, cfg)
	poll.Start()
	defer poll.Stop()

	tray.Run(tray.Options{
		Config:  cfg,
		Auth:    mgr,
		Poll:    poll,
		Snooze:  snooze,
		Notif:   notify.NewNotifier(&cfg.Notifications),
		Updater: updater.NewUpdater(version),
	})
	return nil
}
