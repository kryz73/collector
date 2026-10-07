package database

import "fmt"

const schema = `
CREATE TABLE IF NOT EXISTS channels (
    channel_id      TEXT PRIMARY KEY,
    channel_title   TEXT NOT NULL,
    category_id     INTEGER NOT NULL DEFAULT 0,
    tier            TEXT NOT NULL DEFAULT 'seed',
    last_upload_at  DATETIME,
    added_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    is_active       BOOLEAN NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_channels_active ON channels(is_active);

CREATE TABLE IF NOT EXISTS videos (
    video_id                TEXT PRIMARY KEY,
    channel_id              TEXT NOT NULL,
    channel_title           TEXT NOT NULL DEFAULT '',
    title                   TEXT NOT NULL,
    description             TEXT,
    category_id             INTEGER NOT NULL DEFAULT 0,
    tags                    TEXT,
    duration_seconds        INTEGER DEFAULT 0,
    definition              TEXT DEFAULT 'hd',
    caption                 BOOLEAN DEFAULT 0,
    licensed_content        BOOLEAN DEFAULT 0,
    made_for_kids           BOOLEAN DEFAULT 0,
    live_broadcast_content  TEXT DEFAULT 'none',
    default_audio_lang      TEXT,
    thumbnail_url           TEXT,
    topic_categories        TEXT,
    published_at            DATETIME NOT NULL,
    discovered_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    tracked_from_birth      BOOLEAN NOT NULL DEFAULT 1,
    FOREIGN KEY (channel_id) REFERENCES channels(channel_id)
);
CREATE INDEX IF NOT EXISTS idx_videos_channel ON videos(channel_id);
CREATE INDEX IF NOT EXISTS idx_videos_published ON videos(published_at);

CREATE TABLE IF NOT EXISTS video_tasks (
    video_id            TEXT PRIMARY KEY,
    published_at        DATETIME NOT NULL,
    current_checkpoint  INTEGER NOT NULL DEFAULT 0,
    next_due_at         DATETIME NOT NULL,
    comments_harvested  BOOLEAN NOT NULL DEFAULT 0,
    comments_stage      INTEGER NOT NULL DEFAULT 0,
    is_sealed           BOOLEAN NOT NULL DEFAULT 0,
    sealed_at           DATETIME,
    is_exported         BOOLEAN NOT NULL DEFAULT 0,
    FOREIGN KEY (video_id) REFERENCES videos(video_id)
);
CREATE INDEX IF NOT EXISTS idx_tasks_due ON video_tasks(is_sealed, next_due_at);
CREATE INDEX IF NOT EXISTS idx_tasks_export ON video_tasks(is_sealed, is_exported);

CREATE TABLE IF NOT EXISTS observations (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id                TEXT NOT NULL,
    checkpoint_target_hours REAL NOT NULL,
    actual_elapsed_hours    REAL NOT NULL,
    observed_at             DATETIME NOT NULL,
    view_count              INTEGER NOT NULL DEFAULT 0,
    like_count              INTEGER NOT NULL DEFAULT 0,
    comment_count           INTEGER NOT NULL DEFAULT 0,
    is_trending             BOOLEAN NOT NULL DEFAULT 0,
    FOREIGN KEY (video_id) REFERENCES videos(video_id)
);
CREATE INDEX IF NOT EXISTS idx_obs_video ON observations(video_id);

CREATE TABLE IF NOT EXISTS comments (
    comment_id          TEXT PRIMARY KEY,
    video_id            TEXT NOT NULL,
    author_channel      TEXT,
    author_display_name TEXT,
    text                TEXT NOT NULL,
    published_at        DATETIME NOT NULL,
    updated_at          DATETIME,
    elapsed_minutes     REAL NOT NULL,
    like_count          INTEGER NOT NULL DEFAULT 0,
    reply_count         INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (video_id) REFERENCES videos(video_id)
);
CREATE INDEX IF NOT EXISTS idx_comments_video ON comments(video_id);

CREATE TABLE IF NOT EXISTS trending_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id        TEXT NOT NULL,
    region_code     TEXT NOT NULL,
    trending_rank   INTEGER NOT NULL,
    category_id     INTEGER NOT NULL DEFAULT 0,
    captured_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    is_tracked_seed BOOLEAN NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_trending_video ON trending_events(video_id);
`

func (db *DB) Migrate() error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("executing schema migration: %w", err)
	}

	// Dynamic column migrations for existing databases
	_, _ = db.Exec("ALTER TABLE trending_events ADD COLUMN category_id INTEGER NOT NULL DEFAULT 0;")
	_, _ = db.Exec("ALTER TABLE video_tasks ADD COLUMN comments_stage INTEGER NOT NULL DEFAULT 0;")

	return nil
}
