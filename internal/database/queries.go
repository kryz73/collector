package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// =====================================================================
// Channels
// =====================================================================

func (db *DB) InsertChannel(c Channel) error {
	query := `
	INSERT INTO channels (channel_id, channel_title, category_id, tier, last_upload_at, added_at, is_active)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(channel_id) DO UPDATE SET
		channel_title = excluded.channel_title,
		category_id = CASE WHEN excluded.category_id != 0 THEN excluded.category_id ELSE channels.category_id END,
		is_active = 1;
	`
	_, err := db.Exec(query, c.ChannelID, c.ChannelTitle, c.CategoryID, c.Tier, c.LastUploadAt, c.AddedAt, c.IsActive)
	return err
}

func (db *DB) GetActiveChannels() ([]Channel, error) {
	rows, err := db.Query(`
		SELECT channel_id, channel_title, category_id, tier, last_upload_at, added_at, is_active
		FROM channels WHERE is_active = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ChannelID, &c.ChannelTitle, &c.CategoryID, &c.Tier, &c.LastUploadAt, &c.AddedAt, &c.IsActive); err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

func (db *DB) UpdateChannelLastUpload(channelID string, uploadTime time.Time) error {
	_, err := db.Exec(`
		UPDATE channels 
		SET last_upload_at = ? 
		WHERE channel_id = ? AND (last_upload_at IS NULL OR last_upload_at < ?);
	`, uploadTime, channelID, uploadTime)
	return err
}

func (db *DB) GetChannelCount() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM channels WHERE is_active = 1`).Scan(&count)
	return count, err
}

func (db *DB) EvictOldestChannels(limit int, daysInactive int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -daysInactive)
	_, err := db.Exec(`
		UPDATE channels 
		SET is_active = 0 
		WHERE channel_id IN (
			SELECT channel_id FROM channels 
			WHERE is_active = 1 AND tier = 'trending_discovered' AND (last_upload_at IS NULL OR last_upload_at < ?)
			ORDER BY last_upload_at ASC NULLS FIRST
			LIMIT ?
		);
	`, cutoff, limit)
	return err
}

// =====================================================================
// Videos
// =====================================================================

func (db *DB) InsertVideo(v Video) error {
	query := `
	INSERT INTO videos (
		video_id, channel_id, channel_title, title, description, category_id, tags, 
		duration_seconds, definition, caption, licensed_content, made_for_kids,
		live_broadcast_content, default_audio_lang, thumbnail_url, topic_categories,
		published_at, discovered_at, tracked_from_birth
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(video_id) DO UPDATE SET
		channel_title = excluded.channel_title,
		title = excluded.title,
		description = excluded.description,
		category_id = excluded.category_id,
		tags = excluded.tags,
		duration_seconds = excluded.duration_seconds,
		definition = excluded.definition,
		caption = excluded.caption,
		licensed_content = excluded.licensed_content,
		made_for_kids = excluded.made_for_kids,
		live_broadcast_content = excluded.live_broadcast_content,
		default_audio_lang = excluded.default_audio_lang,
		thumbnail_url = excluded.thumbnail_url,
		topic_categories = excluded.topic_categories;
	`
	_, err := db.Exec(
		query,
		v.VideoID, v.ChannelID, v.ChannelTitle, v.Title, v.Description, v.CategoryID, v.Tags,
		v.DurationSeconds, v.Definition, v.Caption, v.LicensedContent, v.MadeForKids,
		v.LiveBroadcastContent, v.DefaultAudioLang, v.ThumbnailURL, v.TopicCategories,
		v.PublishedAt, v.DiscoveredAt, v.TrackedFromBirth,
	)
	return err
}

func (db *DB) VideoExists(videoID string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM videos WHERE video_id = ?)`, videoID).Scan(&exists)
	return exists, err
}

// =====================================================================
// Tasks (State Machine)
// =====================================================================

func (db *DB) InsertVideoTask(t VideoTask) error {
	query := `
	INSERT INTO video_tasks (
		video_id, published_at, current_checkpoint, next_due_at, comments_harvested, is_sealed, sealed_at, is_exported
	) VALUES (?, ?, ?, ?, ?, ?, ?, 0)
	ON CONFLICT(video_id) DO NOTHING;
	`
	_, err := db.Exec(
		query,
		t.VideoID, t.PublishedAt, t.CurrentCheckpoint, t.NextDueAt, t.CommentsHarvested, t.IsSealed, t.SealedAt,
	)
	return err
}

func (db *DB) GetDueVideoTasks(now time.Time, limit int) ([]VideoTask, error) {
	rows, err := db.Query(`
		SELECT video_id, published_at, current_checkpoint, next_due_at, comments_harvested, is_sealed, sealed_at
		FROM video_tasks
		WHERE is_sealed = 0 AND next_due_at <= ?
		ORDER BY next_due_at ASC
		LIMIT ?;
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []VideoTask
	for rows.Next() {
		var t VideoTask
		if err := rows.Scan(&t.VideoID, &t.PublishedAt, &t.CurrentCheckpoint, &t.NextDueAt, &t.CommentsHarvested, &t.IsSealed, &t.SealedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (db *DB) UpdateTaskCheckpoint(videoID string, nextCheckpoint int, nextDue time.Time, isSealed bool) error {
	var sealedAt *time.Time
	if isSealed {
		now := time.Now().UTC()
		sealedAt = &now
	}
	_, err := db.Exec(`
		UPDATE video_tasks
		SET current_checkpoint = ?, next_due_at = ?, is_sealed = ?, sealed_at = ?
		WHERE video_id = ?;
	`, nextCheckpoint, nextDue, isSealed, sealedAt, videoID)
	return err
}

func (db *DB) MarkCommentsHarvested(videoID string) error {
	_, err := db.Exec(`UPDATE video_tasks SET comments_harvested = 1 WHERE video_id = ?;`, videoID)
	return err
}

func (db *DB) GetSealedUnexportedVideoIDs(limit int) ([]string, error) {
	rows, err := db.Query(`
		SELECT video_id FROM video_tasks
		WHERE is_sealed = 1 AND is_exported = 0
		LIMIT ?;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (db *DB) MarkVideosExported(videoIDs []string) error {
	if len(videoIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`UPDATE video_tasks SET is_exported = 1 WHERE video_id IN (%s);`, strings.Join(placeholders, ","))
	_, err := db.Exec(query, args...)
	return err
}

// =====================================================================
// Observations
// =====================================================================

func (db *DB) InsertObservations(obs []Observation) error {
	if len(obs) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO observations (
			video_id, checkpoint_target_hours, actual_elapsed_hours, 
			observed_at, view_count, like_count, comment_count, is_trending
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, o := range obs {
		if _, err := stmt.Exec(
			o.VideoID, o.CheckpointTargetHours, o.ActualElapsedHours,
			o.ObservedAt, o.ViewCount, o.LikeCount, o.CommentCount, o.IsTrending,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// =====================================================================
// Comments
// =====================================================================

func (db *DB) InsertComments(comments []Comment) error {
	if len(comments) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO comments (
			comment_id, video_id, author_channel, author_display_name, text, 
			published_at, updated_at, elapsed_minutes, like_count, reply_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(comment_id) DO UPDATE SET
			text = excluded.text,
			updated_at = excluded.updated_at,
			like_count = excluded.like_count,
			reply_count = excluded.reply_count;
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, c := range comments {
		if _, err := stmt.Exec(
			c.CommentID, c.VideoID, c.AuthorChannel, c.AuthorDisplayName, c.Text,
			c.PublishedAt, c.UpdatedAt, c.ElapsedMinutes, c.LikeCount, c.ReplyCount,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// =====================================================================
// Trending Events
// =====================================================================

func (db *DB) InsertTrendingEvents(events []TrendingEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO trending_events (video_id, region_code, trending_rank, captured_at, is_tracked_seed)
		VALUES (?, ?, ?, ?, ?);
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range events {
		if _, err := stmt.Exec(e.VideoID, e.RegionCode, e.TrendingRank, e.CapturedAt, e.IsTrackedSeed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) IsVideoCurrentlyTrending(videoID string, withinLastHours float64) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(withinLastHours * float64(time.Hour)))
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM trending_events 
			WHERE video_id = ? AND captured_at >= ?
		)
	`, videoID, cutoff).Scan(&exists)
	return exists, err
}

// =====================================================================
// Export Query Helpers
// =====================================================================

func (db *DB) GetVideosByIDs(videoIDs []string) ([]Video, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT video_id, channel_id, channel_title, title, description, category_id, tags, 
		       duration_seconds, definition, caption, licensed_content, made_for_kids,
		       live_broadcast_content, default_audio_lang, thumbnail_url, topic_categories,
		       published_at, discovered_at, tracked_from_birth
		FROM videos WHERE video_id IN (%s);
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var desc, tags, def, live, lang, thumb, topics sql.NullString
		if err := rows.Scan(
			&v.VideoID, &v.ChannelID, &v.ChannelTitle, &v.Title, &desc, &v.CategoryID, &tags,
			&v.DurationSeconds, &def, &v.Caption, &v.LicensedContent, &v.MadeForKids,
			&live, &lang, &thumb, &topics,
			&v.PublishedAt, &v.DiscoveredAt, &v.TrackedFromBirth,
		); err != nil {
			return nil, err
		}
		v.Description = desc.String
		v.Tags = tags.String
		v.Definition = def.String
		v.LiveBroadcastContent = live.String
		v.DefaultAudioLang = lang.String
		v.ThumbnailURL = thumb.String
		v.TopicCategories = topics.String
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func (db *DB) GetObservationsByVideoIDs(videoIDs []string) ([]Observation, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT id, video_id, checkpoint_target_hours, actual_elapsed_hours, 
		       observed_at, view_count, like_count, comment_count, is_trending
		FROM observations WHERE video_id IN (%s)
		ORDER BY video_id, checkpoint_target_hours ASC;
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []Observation
	for rows.Next() {
		var o Observation
		if err := rows.Scan(
			&o.ID, &o.VideoID, &o.CheckpointTargetHours, &o.ActualElapsedHours,
			&o.ObservedAt, &o.ViewCount, &o.LikeCount, &o.CommentCount, &o.IsTrending,
		); err != nil {
			return nil, err
		}
		observations = append(observations, o)
	}
	return observations, rows.Err()
}

func (db *DB) GetCommentsByVideoIDs(videoIDs []string) ([]Comment, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT comment_id, video_id, author_channel, author_display_name, text, 
		       published_at, updated_at, elapsed_minutes, like_count, reply_count
		FROM comments WHERE video_id IN (%s);
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		var author, authorName sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&c.CommentID, &c.VideoID, &author, &authorName, &c.Text,
			&c.PublishedAt, &updatedAt, &c.ElapsedMinutes, &c.LikeCount, &c.ReplyCount,
		); err != nil {
			return nil, err
		}
		c.AuthorChannel = author.String
		c.AuthorDisplayName = authorName.String
		if updatedAt.Valid {
			c.UpdatedAt = updatedAt.Time
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (db *DB) GetTrendingEventsByVideoIDs(videoIDs []string) ([]TrendingEvent, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(videoIDs))
	args := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT id, video_id, region_code, trending_rank, captured_at, is_tracked_seed
		FROM trending_events WHERE video_id IN (%s);
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TrendingEvent
	for rows.Next() {
		var e TrendingEvent
		if err := rows.Scan(&e.ID, &e.VideoID, &e.RegionCode, &e.TrendingRank, &e.CapturedAt, &e.IsTrackedSeed); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (db *DB) GetAllVideos() ([]Video, error) {
	rows, err := db.Query(`
		SELECT video_id, channel_id, channel_title, title, description, category_id, tags, 
		       duration_seconds, definition, caption, licensed_content, made_for_kids,
		       live_broadcast_content, default_audio_lang, thumbnail_url, topic_categories,
		       published_at, discovered_at, tracked_from_birth
		FROM videos;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var desc, tags, def, live, lang, thumb, topics sql.NullString
		if err := rows.Scan(
			&v.VideoID, &v.ChannelID, &v.ChannelTitle, &v.Title, &desc, &v.CategoryID, &tags,
			&v.DurationSeconds, &def, &v.Caption, &v.LicensedContent, &v.MadeForKids,
			&live, &lang, &thumb, &topics,
			&v.PublishedAt, &v.DiscoveredAt, &v.TrackedFromBirth,
		); err != nil {
			return nil, err
		}
		v.Description = desc.String
		v.Tags = tags.String
		v.Definition = def.String
		v.LiveBroadcastContent = live.String
		v.DefaultAudioLang = lang.String
		v.ThumbnailURL = thumb.String
		v.TopicCategories = topics.String
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func (db *DB) GetAllObservations() ([]Observation, error) {
	rows, err := db.Query(`
		SELECT id, video_id, checkpoint_target_hours, actual_elapsed_hours, 
		       observed_at, view_count, like_count, comment_count, is_trending
		FROM observations
		ORDER BY video_id, checkpoint_target_hours ASC;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []Observation
	for rows.Next() {
		var o Observation
		if err := rows.Scan(
			&o.ID, &o.VideoID, &o.CheckpointTargetHours, &o.ActualElapsedHours,
			&o.ObservedAt, &o.ViewCount, &o.LikeCount, &o.CommentCount, &o.IsTrending,
		); err != nil {
			return nil, err
		}
		observations = append(observations, o)
	}
	return observations, rows.Err()
}

func (db *DB) GetAllComments() ([]Comment, error) {
	rows, err := db.Query(`
		SELECT comment_id, video_id, author_channel, author_display_name, text, 
		       published_at, updated_at, elapsed_minutes, like_count, reply_count
		FROM comments;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		var author, authorName sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(
			&c.CommentID, &c.VideoID, &author, &authorName, &c.Text,
			&c.PublishedAt, &updatedAt, &c.ElapsedMinutes, &c.LikeCount, &c.ReplyCount,
		); err != nil {
			return nil, err
		}
		c.AuthorChannel = author.String
		c.AuthorDisplayName = authorName.String
		if updatedAt.Valid {
			c.UpdatedAt = updatedAt.Time
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (db *DB) GetAllTrendingEvents() ([]TrendingEvent, error) {
	rows, err := db.Query(`
		SELECT id, video_id, region_code, trending_rank, captured_at, is_tracked_seed
		FROM trending_events;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TrendingEvent
	for rows.Next() {
		var e TrendingEvent
		if err := rows.Scan(&e.ID, &e.VideoID, &e.RegionCode, &e.TrendingRank, &e.CapturedAt, &e.IsTrackedSeed); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

