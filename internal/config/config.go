package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	YouTube   YouTubeConfig   `yaml:"youtube"`
	Database  DatabaseConfig  `yaml:"database"`
	Regions   []string        `yaml:"regions"`
	Discovery DiscoveryConfig `yaml:"discovery"`
	Snapshots SnapshotConfig  `yaml:"snapshots"`
	Comments  CommentConfig   `yaml:"comments"`
	Trending  TrendingConfig  `yaml:"trending"`
	Channels  ChannelConfig   `yaml:"channels"`
	Export    ExportConfig    `yaml:"export"`
}

type YouTubeConfig struct {
	APIKey          string `yaml:"api_key"`
	DailyQuotaLimit int    `yaml:"daily_quota_limit"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type DiscoveryConfig struct {
	RSSPollInterval    time.Duration `yaml:"rss_poll_interval"`
	MaxVideoAgeMinutes int           `yaml:"max_video_age_minutes"`
	ConcurrencyLimit   int           `yaml:"concurrency_limit"`
}

type SnapshotConfig struct {
	PollInterval     time.Duration `yaml:"poll_interval"`
	ToleranceMinutes int           `yaml:"tolerance_minutes"`
	CheckpointsHours []float64     `yaml:"checkpoints_hours"`
}

type CommentConfig struct {
	HarvestAtCheckpoint   float64 `yaml:"harvest_at_checkpoint"`
	MaxPagesPerVideo      int     `yaml:"max_pages_per_video"`
	MaxResultsPerPage     int64   `yaml:"max_results_per_page"`
	Stage1CheckpointHours float64 `yaml:"stage_1_checkpoint_hours"`
	Stage1MaxPages        int     `yaml:"stage_1_max_pages"`
	Stage2CheckpointHours float64 `yaml:"stage_2_checkpoint_hours"`
	Stage2MaxPages        int     `yaml:"stage_2_max_pages"`
}

type TrendingConfig struct {
	PollInterval       time.Duration `yaml:"poll_interval"`
	AutoExpandChannels bool          `yaml:"auto_expand_channels"`
	Categories         []int         `yaml:"categories"`
}

type ChannelConfig struct {
	SeedFile             string `yaml:"seed_file"`
	MaxChannels          int    `yaml:"max_channels"`
	EvictionInactiveDays int    `yaml:"eviction_inactive_days"`
}

type ExportConfig struct {
	ParquetInterval time.Duration `yaml:"parquet_interval"`
	OutputDir       string        `yaml:"output_dir"`
}

func DefaultConfig() *Config {
	return &Config{
		YouTube: YouTubeConfig{
			DailyQuotaLimit: 10000,
		},
		Database: DatabaseConfig{
			Path: "./data/harvester.db",
		},
		Regions: []string{"US", "CA", "GB", "AU"},
		Discovery: DiscoveryConfig{
			RSSPollInterval:    60 * time.Minute,
			MaxVideoAgeMinutes: 90,
			ConcurrencyLimit:   25,
		},
		Snapshots: SnapshotConfig{
			PollInterval:     5 * time.Minute,
			ToleranceMinutes: 15,
			CheckpointsHours: []float64{0.5, 1, 1.5, 2, 3, 4, 5, 6, 12, 24, 36, 48, 60},
		},
		Comments: CommentConfig{
			HarvestAtCheckpoint:   6.0,
			MaxPagesPerVideo:      10,
			MaxResultsPerPage:     100,
			Stage1CheckpointHours: 1.0,
			Stage1MaxPages:        5,
			Stage2CheckpointHours: 6.0,
			Stage2MaxPages:        15,
		},
		Trending: TrendingConfig{
			PollInterval:       1 * time.Hour,
			AutoExpandChannels: true,
			Categories:         []int{0, 10, 20, 24},
		},
		Channels: ChannelConfig{
			SeedFile:             "configs/seed_channels.json",
			MaxChannels:          4000,
			EvictionInactiveDays: 60,
		},
		Export: ExportConfig{
			ParquetInterval: 6 * time.Hour,
			OutputDir:       "./data/export",
		},
	}
}

// loadDotEnv loads key-value pairs from .env into the process environment if present.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

func LoadConfig(path string) (*Config, error) {
	// Auto-load .env from working directory or project root
	loadDotEnv(".env")

	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("reading config file %s: %w", path, err)
			}
		} else {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parsing YAML config: %w", err)
			}
		}
	}

	// Environment variable overrides
	if envKey := os.Getenv("YOUTUBE_API_KEY"); envKey != "" {
		cfg.YouTube.APIKey = envKey
	}
	if envDB := os.Getenv("DATABASE_PATH"); envDB != "" {
		cfg.Database.Path = envDB
	}
	if envExport := os.Getenv("EXPORT_DIR"); envExport != "" {
		cfg.Export.OutputDir = envExport
	}

	return cfg, nil
}
