package main

import (
	"context"
	"fmt"
	"log/slog"
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "level",
				Aliases: []string{"l"},
				Value:   "info",
				Usage:   "log level (debug, info, warn, error)",
			},
		},
		Action: run,
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cmd.String("level"))); err != nil {
		return fmt.Errorf("invalid log level %q: %w", cmd.String("level"), err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	snoozePath, err := config.SnoozeFilePath()
	if err != nil {
		return fmt.Errorf("snooze path: %w", err)
	}

	mgr := auth.NewManager()
	snooze := poller.NewSnoozeStore(snoozePath)
	poll := poller.NewManager(mgr, cfg)
	poll.Start()
	defer poll.Stop()

	tray.Run(tray.Options{
		Config:  cfg,
		Auth:    mgr,
		Poll:    poll,
		Snooze:  snooze,
		Notif:   notify.NewNotifier(&cfg.Notifications, snooze),
		Updater: updater.NewUpdater(version),
	})
	return nil
}
