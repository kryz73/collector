package database

import (
	"time"
)

// Channel represents a monitored YouTube channel.
type Channel struct {
	ChannelID    string     `json:"channel_id" db:"channel_id"`
	ChannelTitle string     `json:"channel_title" db:"channel_title"`
	CategoryID   int        `json:"category_id" db:"category_id"`
	Tier         string     `json:"tier" db:"tier"` // "seed", "trending_discovered"
	LastUploadAt *time.Time `json:"last_upload_at" db:"last_upload_at"`
	AddedAt      time.Time  `json:"added_at" db:"added_at"`
	IsActive     bool       `json:"is_active" db:"is_active"`
}

// Video represents comprehensive static video metadata captured upon discovery.
type Video struct {
	VideoID              string    `json:"video_id" db:"video_id" parquet:"video_id"`
	ChannelID            string    `json:"channel_id" db:"channel_id" parquet:"channel_id"`
	ChannelTitle         string    `json:"channel_title" db:"channel_title" parquet:"channel_title"`
	Title                string    `json:"title" db:"title" parquet:"title"`
	Description          string    `json:"description" db:"description" parquet:"description"`
	CategoryID           int       `json:"category_id" db:"category_id" parquet:"category_id"`
	Tags                 string    `json:"tags" db:"tags" parquet:"tags"` // JSON array string
	DurationSeconds      int       `json:"duration_seconds" db:"duration_seconds" parquet:"duration_seconds"`
	Definition           string    `json:"definition" db:"definition" parquet:"definition"`                       // "hd" vs "sd"
	Caption              bool      `json:"caption" db:"caption" parquet:"caption"`                               // closed captions available
	LicensedContent      bool      `json:"licensed_content" db:"licensed_content" parquet:"licensed_content"`     // licensed/MCN content
	MadeForKids          bool      `json:"made_for_kids" db:"made_for_kids" parquet:"made_for_kids"`             // COPPA kids content
	LiveBroadcastContent string    `json:"live_broadcast_content" db:"live_broadcast_content" parquet:"live_broadcast_content"` // "none", "live", "upcoming"
	DefaultAudioLang     string    `json:"default_audio_lang" db:"default_audio_lang" parquet:"default_audio_lang"`
	ThumbnailURL         string    `json:"thumbnail_url" db:"thumbnail_url" parquet:"thumbnail_url"`
	TopicCategories      string    `json:"topic_categories" db:"topic_categories" parquet:"topic_categories"`   // JSON array of Wikipedia topic URLs
	PublishedAt          time.Time `json:"published_at" db:"published_at" parquet:"published_at"`
	DiscoveredAt         time.Time `json:"discovered_at" db:"discovered_at" parquet:"discovered_at"`
	TrackedFromBirth     bool      `json:"tracked_from_birth" db:"tracked_from_birth" parquet:"tracked_from_birth"`
}

// VideoTask represents a video in the active 60-hour state machine tracking queue.
type VideoTask struct {
	VideoID           string     `json:"video_id" db:"video_id"`
	PublishedAt       time.Time  `json:"published_at" db:"published_at"`
	CurrentCheckpoint int        `json:"current_checkpoint" db:"current_checkpoint"`
	NextDueAt         time.Time  `json:"next_due_at" db:"next_due_at"`
	CommentsHarvested bool       `json:"comments_harvested" db:"comments_harvested"`
	CommentsStage     int        `json:"comments_stage" db:"comments_stage"` // 0=none, 1=stage 1 (1h), 2=stage 2 (6h)
	IsSealed          bool       `json:"is_sealed" db:"is_sealed"`
	SealedAt          *time.Time `json:"sealed_at" db:"sealed_at"`
}

// Observation represents a time-series engagement snapshot at a specific checkpoint.
type Observation struct {
	ID                    int64     `json:"id" db:"id" parquet:"id"`
	VideoID               string    `json:"video_id" db:"video_id" parquet:"video_id"`
	CheckpointTargetHours float64   `json:"checkpoint_target_hours" db:"checkpoint_target_hours" parquet:"checkpoint_target_hours"`
	ActualElapsedHours    float64   `json:"actual_elapsed_hours" db:"actual_elapsed_hours" parquet:"actual_elapsed_hours"`
	ObservedAt            time.Time `json:"observed_at" db:"observed_at" parquet:"observed_at"`
	ViewCount             int64     `json:"view_count" db:"view_count" parquet:"view_count"`
	LikeCount             int64     `json:"like_count" db:"like_count" parquet:"like_count"`
	CommentCount          int64     `json:"comment_count" db:"comment_count" parquet:"comment_count"`
	IsTrending            bool      `json:"is_trending" db:"is_trending" parquet:"is_trending"`
}

// Comment represents an audience comment captured within the early observation window.
type Comment struct {
	CommentID         string    `json:"comment_id" db:"comment_id" parquet:"comment_id"`
	VideoID           string    `json:"video_id" db:"video_id" parquet:"video_id"`
	AuthorChannel     string    `json:"author_channel" db:"author_channel" parquet:"author_channel"`
	AuthorDisplayName string    `json:"author_display_name" db:"author_display_name" parquet:"author_display_name"`
	Text              string    `json:"text" db:"text" parquet:"text"` // raw unescaped text (TextOriginal)
	PublishedAt       time.Time `json:"published_at" db:"published_at" parquet:"published_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at" parquet:"updated_at"`
	ElapsedMinutes    float64   `json:"elapsed_minutes" db:"elapsed_minutes" parquet:"elapsed_minutes"`
	LikeCount         int64     `json:"like_count" db:"like_count" parquet:"like_count"`
	ReplyCount        int64     `json:"reply_count" db:"reply_count" parquet:"reply_count"`
}

// TrendingEvent represents a ground-truth trending observation from mostPopular charts.
type TrendingEvent struct {
	ID            int64     `json:"id" db:"id" parquet:"id"`
	VideoID       string    `json:"video_id" db:"video_id" parquet:"video_id"`
	RegionCode    string    `json:"region_code" db:"region_code" parquet:"region_code"`
	TrendingRank  int       `json:"trending_rank" db:"trending_rank" parquet:"trending_rank"`
	CategoryID    int       `json:"category_id" db:"category_id" parquet:"category_id"`
	CapturedAt    time.Time `json:"captured_at" db:"captured_at" parquet:"captured_at"`
	IsTrackedSeed bool      `json:"is_tracked_seed" db:"is_tracked_seed" parquet:"is_tracked_seed"`
}
