package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/kryz73/collector/internal/database"
	"github.com/kryz73/collector/internal/quota"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	yapi "google.golang.org/api/youtube/v3"
)

type Client struct {
	service  *yapi.Service
	governor *quota.Governor
	logger   *slog.Logger
}

func NewClient(ctx context.Context, apiKey string, gov *quota.Governor, logger *slog.Logger) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("youtube API key must not be empty")
	}

	service, err := yapi.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("creating youtube service: %w", err)
	}

	return &Client{
		service:  service,
		governor: gov,
		logger:   logger,
	}, nil
}

// FetchVideoDetails fetches snippet, statistics, and contentDetails for up to 50 video IDs per call.
func (c *Client) FetchVideoDetails(ctx context.Context, videoIDs []string) ([]database.Video, []database.Observation, error) {
	if len(videoIDs) == 0 {
		return nil, nil, nil
	}

	var allVideos []database.Video
	var allObs []database.Observation
	now := time.Now().UTC()

	// Chunk IDs into batches of up to 50 (API limit)
	for i := 0; i < len(videoIDs); i += 50 {
		end := i + 50
		if end > len(videoIDs) {
			end = len(videoIDs)
		}
		chunk := videoIDs[i:end]
		joinedIDs := strings.Join(chunk, ",")

		if !c.governor.Spend(1, quota.PriorityCritical) {
			return allVideos, allObs, fmt.Errorf("quota exceeded for videos.list")
		}

		var response *yapi.VideoListResponse
		err := c.retryWithBackoff(ctx, func() error {
			call := c.service.Videos.List([]string{"snippet", "statistics", "contentDetails"}).
				Id(joinedIDs).
				Context(ctx)
			var apiErr error
			response, apiErr = call.Do()
			return apiErr
		})

		if err != nil {
			c.logger.Error("videos.list API error", "error", err, "batch_size", len(chunk))
			return allVideos, allObs, fmt.Errorf("videos.list batch failed: %w", err)
		}

		for _, item := range response.Items {
			pubTime, parseErr := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			if parseErr != nil {
				pubTime = now
			}

			catID, _ := strconv.Atoi(item.Snippet.CategoryId)
			tagsJSON, _ := json.Marshal(item.Snippet.Tags)
			durationSecs := ParseISODuration(item.ContentDetails.Duration)

			v := database.Video{
				VideoID:          item.Id,
				ChannelID:        item.Snippet.ChannelId,
				Title:            item.Snippet.Title,
				Description:      item.Snippet.Description,
				CategoryID:       catID,
				Tags:             string(tagsJSON),
				DurationSeconds:  durationSecs,
				DefaultAudioLang: item.Snippet.DefaultAudioLanguage,
				PublishedAt:      pubTime,
				DiscoveredAt:     now,
				TrackedFromBirth: true,
			}
			allVideos = append(allVideos, v)

			elapsedHours := now.Sub(pubTime).Hours()
			obs := database.Observation{
				VideoID:                item.Id,
				CheckpointTargetHours:  elapsedHours,
				ActualElapsedHours:     elapsedHours,
				ObservedAt:             now,
				ViewCount:              int64(item.Statistics.ViewCount),
				LikeCount:              int64(item.Statistics.LikeCount),
				CommentCount:           int64(item.Statistics.CommentCount),
				IsTrending:             false,
			}
			allObs = append(allObs, obs)
		}
	}

	return allVideos, allObs, nil
}

// FetchTrending fetches top 50 trending videos for a specified country code (e.g. "US", "CA").
func (c *Client) FetchTrending(ctx context.Context, regionCode string) ([]database.TrendingEvent, []database.Video, error) {
	if !c.governor.Spend(1, quota.PriorityCritical) {
		return nil, nil, fmt.Errorf("quota exceeded for chart=mostPopular")
	}

	var response *yapi.VideoListResponse
	err := c.retryWithBackoff(ctx, func() error {
		call := c.service.Videos.List([]string{"snippet", "statistics", "contentDetails"}).
			Chart("mostPopular").
			RegionCode(regionCode).
			MaxResults(50).
			Context(ctx)
		var apiErr error
		response, apiErr = call.Do()
		return apiErr
	})

	if err != nil {
		return nil, nil, fmt.Errorf("chart=mostPopular error for region %s: %w", regionCode, err)
	}

	now := time.Now().UTC()
	var events []database.TrendingEvent
	var videos []database.Video

	for rank, item := range response.Items {
		pubTime, parseErr := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
		if parseErr != nil {
			pubTime = now
		}

		catID, _ := strconv.Atoi(item.Snippet.CategoryId)
		tagsJSON, _ := json.Marshal(item.Snippet.Tags)
		durationSecs := ParseISODuration(item.ContentDetails.Duration)

		events = append(events, database.TrendingEvent{
			VideoID:       item.Id,
			RegionCode:    regionCode,
			TrendingRank:  rank + 1,
			CapturedAt:    now,
			IsTrackedSeed: false, // Updated by caller if video was tracked
		})

		videos = append(videos, database.Video{
			VideoID:          item.Id,
			ChannelID:        item.Snippet.ChannelId,
			Title:            item.Snippet.Title,
			Description:      item.Snippet.Description,
			CategoryID:       catID,
			Tags:             string(tagsJSON),
			DurationSeconds:  durationSecs,
			DefaultAudioLang: item.Snippet.DefaultAudioLanguage,
			PublishedAt:      pubTime,
			DiscoveredAt:     now,
			TrackedFromBirth: false,
		})
	}

	return events, videos, nil
}

// FetchComments fetches paginated comments for a video, stopping at early cutoff or maxPages.
func (c *Client) FetchComments(ctx context.Context, videoID string, videoPublishedAt time.Time, maxPages int) ([]database.Comment, error) {
	var comments []database.Comment
	pageToken := ""

	for page := 0; page < maxPages; page++ {
		if !c.governor.Spend(1, quota.PriorityLow) {
			c.logger.Warn("Quota governor paused comment harvesting", "video_id", videoID, "page", page)
			break
		}

		call := c.service.CommentThreads.List([]string{"snippet"}).
			VideoId(videoID).
			Order("time").
			MaxResults(100).
			TextFormat("plainText").
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		var response *yapi.CommentThreadListResponse
		err := c.retryWithBackoff(ctx, func() error {
			var apiErr error
			response, apiErr = call.Do()
			return apiErr
		})

		if err != nil {
			// Check if comments are disabled on the video
			if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == 403 {
				for _, item := range gErr.Errors {
					if item.Reason == "commentsDisabled" {
						c.logger.Debug("Comments disabled for video", "video_id", videoID)
						return comments, nil
					}
				}
			}
			return comments, fmt.Errorf("commentThreads.list error on video %s: %w", videoID, err)
		}

		for _, item := range response.Items {
			top := item.Snippet.TopLevelComment.Snippet
			cPubTime, parseErr := time.Parse(time.RFC3339, top.PublishedAt)
			if parseErr != nil {
				cPubTime = time.Now().UTC()
			}

			elapsedMins := cPubTime.Sub(videoPublishedAt).Minutes()

			// Anti-leakage: comments strictly within 6-hour window (360 minutes)
			if elapsedMins > 360.0 {
				continue
			}

			authorChannel := ""
			if top.AuthorChannelId != nil {
				authorChannel = top.AuthorChannelId.Value
			}

			comments = append(comments, database.Comment{
				CommentID:      item.Id,
				VideoID:        videoID,
				AuthorChannel:  authorChannel,
				Text:           top.TextDisplay,
				PublishedAt:    cPubTime,
				ElapsedMinutes: elapsedMins,
				LikeCount:      top.LikeCount,
				ReplyCount:     item.Snippet.TotalReplyCount,
			})
		}

		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return comments, nil
}

// retryWithBackoff retries transient 5xx or network errors with exponential backoff and jitter.
func (c *Client) retryWithBackoff(ctx context.Context, op func() error) error {
	maxRetries := 4
	baseDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := op()
		if err == nil {
			return nil
		}

		// Don't retry non-transient 4xx errors (e.g. 403 commentsDisabled, 404 videoNotFound)
		if gErr, ok := err.(*googleapi.Error); ok {
			if gErr.Code >= 400 && gErr.Code < 500 {
				return err
			}
		}

		if attempt == maxRetries {
			return err
		}

		jitter := time.Duration(rand.Intn(500)) * time.Millisecond
		delay := (baseDelay << attempt) + jitter

		c.logger.Warn("Transient API error, retrying with backoff",
			"attempt", attempt+1,
			"delay", delay,
			"error", err,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil
}
