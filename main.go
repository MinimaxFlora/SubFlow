// Command subflow is the SubFlow server: a subscription management,
// node processing and configuration generation platform.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/subflow-org/subflow/internal/api"
	"github.com/subflow-org/subflow/internal/config"
	"github.com/subflow-org/subflow/internal/fetch"
	"github.com/subflow-org/subflow/internal/storage"
	"github.com/subflow-org/subflow/internal/version"
)

//go:embed all:web/dist
var embeddedFrontend embed.FS

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "subflow:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if wantsVersion(args) {
		info := version.Get()
		fmt.Printf("subflow %s (commit %s, built %s)\n", info.Version, info.Commit, info.BuildTime)
		return nil
	}

	cfg, err := config.Load(args)
	if err != nil {
		return err
	}

	setupLogger(cfg.LogLevel)
	info := version.Get()
	slog.Info("starting subflow",
		"version", info.Version,
		"commit", info.Commit,
		"dataDir", cfg.DataDir,
		"db", cfg.DBPath,
		"dev", cfg.Dev,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			slog.Error("close database", "error", cerr)
		}
	}()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	slog.Info("database ready", "path", cfg.DBPath)

	fetcher := fetch.New(fetch.Options{
		UserAgent:         cfg.FetchUserAgent,
		AllowPrivateHosts: cfg.FetchAllowPrivateHosts,
	})

	server := api.New(api.Deps{
		Subscriptions: storage.NewSubscriptionRepo(db),
		Settings:      storage.NewSettingsRepo(db),
		Resources:     storage.NewResourceRepo(db),
		Fetcher:       fetcher,
		Assets:        frontendFS(),
		Dev:           cfg.Dev,
	})

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if err := server.Run(ctx, addr); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	slog.Info("subflow stopped")
	return nil
}

// frontendFS returns the embedded frontend rooted at web/dist.
func frontendFS() fs.FS {
	sub, err := fs.Sub(embeddedFrontend, "web/dist")
	if err != nil {
		slog.Warn("frontend assets unavailable", "error", err)
		return nil
	}
	return sub
}

func wantsVersion(args []string) bool {
	for _, a := range args {
		if a == "version" || a == "--version" || a == "-version" {
			return true
		}
	}
	return false
}

func setupLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
