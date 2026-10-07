package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kryz73/harvest/internal/config"
	"github.com/kryz73/harvest/internal/database"
	"github.com/kryz73/harvest/internal/discovery"
	"github.com/kryz73/harvest/internal/export"
	"github.com/kryz73/harvest/internal/quota"
	"github.com/kryz73/harvest/internal/registry"
	"github.com/kryz73/harvest/internal/youtube"
)

type Scheduler struct {
	cfg        *config.Config
	db         *database.DB
	ytClient   *youtube.Client
	poller     *discovery.Poller
	regManager *registry.Manager
	exporter   *export.Exporter
	governor   *quota.Governor
	logger     *slog.Logger
}

func NewScheduler(
	cfg *config.Config,
	db *database.DB,
	ytClient *youtube.Client,
	poller *discovery.Poller,
	regManager *registry.Manager,
	exporter *export.Exporter,
	governor *quota.Governor,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		cfg:        cfg,
		db:         db,
		ytClient:   ytClient,
		poller:     poller,
		regManager: regManager,
		exporter:   exporter,
		governor:   governor,
		logger:     logger,
	}
}

// Start launches the 4 concurrent scheduler loops and blocks until ctx is canceled.
func (s *Scheduler) Start(ctx context.Context) {
	var wg sync.WaitGroup

	s.logger.Info("Starting YouTube Data Harvester schedulers",
		"rss_poll_interval", s.cfg.Discovery.RSSPollInterval,
		"snapshot_poll_interval", s.cfg.Snapshots.PollInterval,
		"trending_poll_interval", s.cfg.Trending.PollInterval,
		"export_interval", s.cfg.Export.ParquetInterval,
	)

	// Run initial discovery, snapshot, and trending check on startup
	s.runDiscoveryOnce(ctx)
	s.runSnapshotOnce(ctx)
	s.runTrendingOnce(ctx)

	// Loop 1: Channel Discovery (RSS)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(s.cfg.Discovery.RSSPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDiscoveryOnce(ctx)
			}
		}
	}()

	// Loop 2: Observation Snapshots & Comments
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(s.cfg.Snapshots.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runSnapshotOnce(ctx)
			}
		}
	}()

	// Loop 3: Trending Chart Monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(s.cfg.Trending.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runTrendingOnce(ctx)
			}
		}
	}()

	// Loop 4: Parquet Exporter
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(s.cfg.Export.ParquetInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runExportOnce()
			}
		}
	}()

	wg.Wait()
	s.logger.Info("All harvester loops stopped gracefully")
}

// =====================================================================
// Job 1: Channel Discovery via 0-Quota RSS Feeds
// =====================================================================

func (s *Scheduler) runDiscoveryOnce(ctx context.Context) {
	channels, err := s.db.GetActiveChannels()
	if err != nil {
		s.logger.Error("Failed fetching active channels for RSS poll", "error", err)
		return
	}

	if len(channels) == 0 {
		return
	}

	discovered := s.poller.PollAllChannels(ctx, channels, s.cfg.Discovery.MaxVideoAgeMinutes)
	if len(discovered) == 0 {
		return
	}

	var newVideoIDs []string

	for _, dv := range discovered {
		exists, err := s.db.VideoExists(dv.VideoID)
		if err != nil || exists {
			continue
		}
		newVideoIDs = append(newVideoIDs, dv.VideoID)
		_ = s.db.UpdateChannelLastUpload(dv.ChannelID, dv.PublishedAt)
	}

	if len(newVideoIDs) == 0 {
		return
	}

	s.logger.Info("New video uploads discovered via RSS", "count", len(newVideoIDs))

	// Fetch full metadata for newly discovered videos (batched up to 50)
	videos, _, err := s.ytClient.FetchVideoDetails(ctx, newVideoIDs)
	if err != nil {
		s.logger.Error("Failed fetching video details for new uploads", "error", err)
		return
	}

	for _, v := range videos {
		if err := s.db.InsertVideo(v); err != nil {
			s.logger.Warn("Failed inserting new video", "video_id", v.VideoID, "error", err)
			continue
		}

		// First checkpoint is scheduled based on checkpoints_hours (defaulting to first configured mark)
		firstHours := 1.0
		if len(s.cfg.Snapshots.CheckpointsHours) > 0 {
			firstHours = s.cfg.Snapshots.CheckpointsHours[0]
		}
		nextDue := v.PublishedAt.Add(time.Duration(firstHours * float64(time.Hour)))
		task := database.VideoTask{
			VideoID:           v.VideoID,
			PublishedAt:       v.PublishedAt,
			CurrentCheckpoint: 0,
			NextDueAt:         nextDue,
			CommentsHarvested: false,
			CommentsStage:     0,
			IsSealed:          false,
		}
		if err := s.db.InsertVideoTask(task); err != nil {
			s.logger.Warn("Failed inserting video task", "video_id", v.VideoID, "error", err)
		}
	}

	s.logger.Info("New videos registered and queued for tracking", "count", len(videos))
}

// =====================================================================
// Job 2: Video Observation Snapshots & Comment Harvesting
// =====================================================================

func (s *Scheduler) runSnapshotOnce(ctx context.Context) {
	now := time.Now().UTC()
	dueTasks, err := s.db.GetDueVideoTasks(now, 100) // Process up to 100 due tasks per tick
	if err != nil {
		s.logger.Error("Failed querying due video tasks", "error", err)
		return
	}

	if len(dueTasks) == 0 {
		return
	}

	var videoIDs []string
	taskMap := make(map[string]database.VideoTask)
	for _, t := range dueTasks {
		videoIDs = append(videoIDs, t.VideoID)
		taskMap[t.VideoID] = t
	}

	// Batch fetch snapshots (up to 50 IDs per call)
	_, obsList, err := s.ytClient.FetchVideoDetails(ctx, videoIDs)
	if err != nil {
		s.logger.Error("Failed batch fetching observation details", "error", err)
		return
	}

	// Update observations with trending cross-reference
	for i := range obsList {
		isTrending, _ := s.db.IsVideoCurrentlyTrending(obsList[i].VideoID, 6.0)
		obsList[i].IsTrending = isTrending

		task, exists := taskMap[obsList[i].VideoID]
		if exists {
			targetHours := s.getCheckpointHours(task.CurrentCheckpoint + 1)
			obsList[i].CheckpointTargetHours = targetHours
			obsList[i].ActualElapsedHours = now.Sub(task.PublishedAt).Hours()
		}
	}

	if err := s.db.InsertObservations(obsList); err != nil {
		s.logger.Error("Failed inserting observations", "error", err)
		return
	}

	// Advance checkpoints and trigger comment harvesting
	checkpoints := s.cfg.Snapshots.CheckpointsHours
	for _, obs := range obsList {
		task := taskMap[obs.VideoID]
		nextIndex := task.CurrentCheckpoint + 1

		// Dual-stage comment harvesting:
		targetHours := s.getCheckpointHours(nextIndex)

		// Stage 1: early reaction surge (e.g. 1.0h, max 5 pages)
		stage1Hours := s.cfg.Comments.Stage1CheckpointHours
		if stage1Hours <= 0 {
			stage1Hours = 1.0
		}
		if targetHours >= stage1Hours && task.CommentsStage < 1 {
			s.harvestCommentsForVideoStage(ctx, task.VideoID, task.PublishedAt, 1, s.cfg.Comments.Stage1MaxPages)
			task.CommentsStage = 1
		}

		// Stage 2: full discussion maturation (e.g. 6.0h, max 15 pages)
		stage2Hours := s.cfg.Comments.Stage2CheckpointHours
		if stage2Hours <= 0 {
			stage2Hours = 6.0
		}
		if targetHours >= stage2Hours && task.CommentsStage < 2 {
			s.harvestCommentsForVideoStage(ctx, task.VideoID, task.PublishedAt, 2, s.cfg.Comments.Stage2MaxPages)
			task.CommentsStage = 2
		}

		if nextIndex >= len(checkpoints) {
			// Video has completed all checkpoints (60 hours) -> Seal it
			_ = s.db.UpdateTaskCheckpoint(task.VideoID, nextIndex, now, true)
			s.logger.Info("Video lifecycle completed and sealed", "video_id", task.VideoID)
		} else {
			nextTarget := checkpoints[nextIndex]
			nextDue := task.PublishedAt.Add(time.Duration(nextTarget * float64(time.Hour)))
			_ = s.db.UpdateTaskCheckpoint(task.VideoID, nextIndex, nextDue, false)
		}
	}

	s.logger.Info("Recorded engagement snapshots",
		"count", len(obsList),
		"quota_used_today", s.governor.Used(),
		"quota_remaining", s.governor.Remaining(),
	)
}

func (s *Scheduler) harvestCommentsForVideoStage(ctx context.Context, videoID string, publishedAt time.Time, stage int, pageLimit int) {
	if pageLimit <= 0 {
		pageLimit = 10
	}
	maxPages := s.governor.MaxCommentPages(pageLimit)
	comments, err := s.ytClient.FetchComments(ctx, videoID, publishedAt, maxPages)
	if err != nil {
		s.logger.Warn("Failed harvesting comments for video", "video_id", videoID, "stage", stage, "error", err)
	} else if len(comments) > 0 {
		if err := s.db.InsertComments(comments); err != nil {
			s.logger.Error("Failed saving harvested comments", "video_id", videoID, "stage", stage, "error", err)
		} else {
			s.logger.Info("Harvested comments", "video_id", videoID, "stage", stage, "count", len(comments))
		}
	}
	_ = s.db.UpdateTaskCommentsStage(videoID, stage)
}

func (s *Scheduler) harvestCommentsForVideo(ctx context.Context, videoID string, publishedAt time.Time) {
	s.harvestCommentsForVideoStage(ctx, videoID, publishedAt, 2, s.cfg.Comments.Stage2MaxPages)
}

func (s *Scheduler) getCheckpointHours(index int) float64 {
	checkpoints := s.cfg.Snapshots.CheckpointsHours
	if index <= 0 {
		return 0
	}
	if index > len(checkpoints) {
		return checkpoints[len(checkpoints)-1]
	}
	return checkpoints[index-1]
}

// =====================================================================
// Job 3: Trending Chart Monitor & Auto-Channel Expansion
// =====================================================================

func (s *Scheduler) runTrendingOnce(ctx context.Context) {
	categories := s.cfg.Trending.Categories
	if len(categories) == 0 {
		categories = []int{0}
	}

	for _, region := range s.cfg.Regions {
		for _, catID := range categories {
			events, videos, err := s.ytClient.FetchTrending(ctx, region, catID)
			if err != nil {
				s.logger.Error("Failed fetching trending chart", "region", region, "category_id", catID, "error", err)
				continue
			}

			for i := range events {
				exists, _ := s.db.VideoExists(events[i].VideoID)
				events[i].IsTrackedSeed = exists
			}

			if err := s.db.InsertTrendingEvents(events); err != nil {
				s.logger.Error("Failed saving trending events", "region", region, "category_id", catID, "error", err)
			}

			// Handle untracked trending videos: ingest and auto-expand channels
			for _, v := range videos {
				exists, _ := s.db.VideoExists(v.VideoID)
				if !exists {
					// 1. Ensure channel is in channels table first (satisfies FOREIGN KEY)
					if s.cfg.Trending.AutoExpandChannels {
						if err := s.regManager.AddDiscoveredChannel(v.ChannelID, v.ChannelTitle, v.CategoryID); err != nil {
							s.logger.Warn("Failed auto-expanding trending channel", "channel_id", v.ChannelID, "error", err)
						}
					} else {
						// Insert stub channel if auto-expand is disabled to satisfy FK
						_ = s.db.InsertChannel(database.Channel{
							ChannelID:    v.ChannelID,
							ChannelTitle: v.ChannelTitle,
							CategoryID:   v.CategoryID,
							Tier:         "trending_stub",
							AddedAt:      time.Now().UTC(),
							IsActive:     false,
						})
					}

					// 2. Insert video
					v.TrackedFromBirth = false
					if err := s.db.InsertVideo(v); err != nil {
						s.logger.Error("Failed inserting trending video", "video_id", v.VideoID, "error", err)
						continue
					}

					// 3. Harvest early comments for this viral video
					s.harvestCommentsForVideo(ctx, v.VideoID, v.PublishedAt)
				}
			}

			s.logger.Info("Processed trending chart", "region", region, "category_id", catID, "entries", len(events))
		}
	}
}

// =====================================================================
// Job 4: Parquet Data Lake Exporter
// =====================================================================

func (s *Scheduler) runExportOnce() {
	exportedCount, err := s.exporter.ExportSealedVideos(200)
	if err != nil {
		s.logger.Error("Parquet export job failed", "error", err)
		return
	}
	if exportedCount > 0 {
		s.logger.Info("Parquet export cycle completed", "videos_exported", exportedCount)
	}
}
