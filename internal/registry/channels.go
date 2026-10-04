package registry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kryz73/collector/internal/database"
)

type SeedFile struct {
	Description string `json:"description"`
	Tiers       map[string]struct {
		TargetSubscriberRange string `json:"target_subscriber_range"`
		ExpectedTrendingRate  float64 `json:"expected_trending_rate"`
		Channels              []struct {
			ChannelID  string `json:"channel_id"`
			Name       string `json:"name"`
			CategoryID int    `json:"category_id"`
		} `json:"channels"`
	} `json:"tiers"`
}

type Manager struct {
	db                   *database.DB
	maxChannels          int
	evictionInactiveDays int
	logger               *slog.Logger
}

func NewManager(db *database.DB, maxChannels int, evictionDays int, logger *slog.Logger) *Manager {
	return &Manager{
		db:                   db,
		maxChannels:          maxChannels,
		evictionInactiveDays: evictionDays,
		logger:               logger,
	}
}

// LoadSeedChannels loads seed channels from JSON into the database if the table is empty.
func (m *Manager) LoadSeedChannels(seedFilePath string) error {
	count, err := m.db.GetChannelCount()
	if err != nil {
		return fmt.Errorf("checking channel count: %w", err)
	}

	if count > 0 {
		m.logger.Info("Channel registry already populated", "active_count", count)
		return nil
	}

	data, err := os.ReadFile(seedFilePath)
	if err != nil {
		return fmt.Errorf("reading seed channels file %s: %w", seedFilePath, err)
	}

	var sf SeedFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return fmt.Errorf("parsing seed channels JSON: %w", err)
	}

	now := time.Now().UTC()
	loaded := 0

	for tierName, tier := range sf.Tiers {
		for _, c := range tier.Channels {
			ch := database.Channel{
				ChannelID:    c.ChannelID,
				ChannelTitle: c.Name,
				CategoryID:   c.CategoryID,
				Tier:         tierName,
				AddedAt:      now,
				IsActive:     true,
			}
			if err := m.db.InsertChannel(ch); err != nil {
				m.logger.Warn("Failed inserting seed channel", "channel_id", c.ChannelID, "error", err)
			} else {
				loaded++
			}
		}
	}

	m.logger.Info("Seed channels successfully loaded into database", "count", loaded)
	return nil
}

// AddDiscoveredChannel adds an untracked channel discovered via trending charts with LRU eviction.
func (m *Manager) AddDiscoveredChannel(channelID, channelTitle string, categoryID int) error {
	count, err := m.db.GetChannelCount()
	if err != nil {
		return err
	}

	// LRU eviction if maximum cap reached
	if count >= m.maxChannels {
		if err := m.db.EvictOldestChannels(1, m.evictionInactiveDays); err != nil {
			m.logger.Warn("Failed evicting inactive channel", "error", err)
		} else {
			m.logger.Info("Evicted least active channel to maintain channel cap", "cap", m.maxChannels)
		}
	}

	ch := database.Channel{
		ChannelID:    channelID,
		ChannelTitle: channelTitle,
		CategoryID:   categoryID,
		Tier:         "trending_discovered",
		AddedAt:      time.Now().UTC(),
		IsActive:     true,
	}

	return m.db.InsertChannel(ch)
}
