package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kryz73/harvest/internal/config"
	"github.com/kryz73/harvest/internal/database"
	"github.com/kryz73/harvest/internal/discovery"
	"github.com/kryz73/harvest/internal/export"
	"github.com/kryz73/harvest/internal/quota"
	"github.com/kryz73/harvest/internal/registry"
	"github.com/kryz73/harvest/internal/scheduler"
	"github.com/kryz73/harvest/internal/youtube"
)

func main() {
	configPath := flag.String("config", "configs/harvester.yaml", "Path to YAML configuration file")
	flag.Parse()

	// 1. Structured Logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Initializing YouTube Data Harvester (harvest/)")

	// 2. Load Configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Error("Failed loading configuration", "error", err)
		os.Exit(1)
	}

	if cfg.YouTube.APIKey == "" {
		logger.Error("YOUTUBE_API_KEY environment variable or youtube.api_key in config is required")
		os.Exit(1)
	}

	// 3. Initialize SQLite Database
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		logger.Error("Failed opening SQLite database", "path", cfg.Database.Path, "error", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("Database initialized with WAL mode", "path", cfg.Database.Path)

	// 4. Initialize Quota Governor (10,000 units/day)
	gov := quota.NewGovernor(cfg.YouTube.DailyQuotaLimit, logger)
	logger.Info("Quota Governor active", "daily_limit", cfg.YouTube.DailyQuotaLimit)

	// 5. Initialize YouTube API Client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ytClient, err := youtube.NewClient(ctx, cfg.YouTube.APIKey, gov, logger)
	if err != nil {
		logger.Error("Failed creating YouTube client", "error", err)
		os.Exit(1)
	}

	// 6. Initialize Registry, Poller, and Exporter
	regManager := registry.NewManager(db, cfg.Channels.MaxChannels, cfg.Channels.EvictionInactiveDays, logger)
	if err := regManager.LoadSeedChannels(cfg.Channels.SeedFile); err != nil {
		logger.Warn("Failed loading initial seed channels", "error", err)
	}

	poller := discovery.NewPoller(cfg.Discovery.ConcurrencyLimit, logger)
	exporter := export.NewExporter(db, cfg.Export.OutputDir, logger)

	// 7. Initialize and Start Scheduler
	sched := scheduler.NewScheduler(cfg, db, ytClient, poller, regManager, exporter, gov, logger)

	// 8. Graceful Shutdown on OS Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutdown signal received, stopping harvester...")
		cancel()
	}()

	fmt.Println("==================================================================")
	fmt.Println(" YouTube Data Harvester running 24/7 on AWS EC2")
	fmt.Println(" Zero-Cost RSS Discovery + 60h Batched Lifecycle Tracking")
	fmt.Println("==================================================================")

	sched.Start(ctx)

	// Wait 1 second for any remaining I/O to flush
	time.Sleep(1 * time.Second)
	logger.Info("YouTube Data Harvester terminated cleanly")
}
