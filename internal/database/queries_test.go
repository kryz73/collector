package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "harvester_test_*")
	if err != nil {
		t.Fatalf("Failed creating temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed opening test DB: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestDatabaseLifecycle(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 1. Channel insertion
	now := time.Now().UTC()
	ch := Channel{
		ChannelID:    "UCtest123",
		ChannelTitle: "Test Channel",
		CategoryID:   28,
		Tier:         "seed",
		AddedAt:      now,
		IsActive:     true,
	}
	if err := db.InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel failed: %v", err)
	}

	count, err := db.GetChannelCount()
	if err != nil || count != 1 {
		t.Errorf("Expected channel count 1, got %d (err: %v)", count, err)
	}

	// 2. Video insertion
	v := Video{
		VideoID:          "vid123",
		ChannelID:        "UCtest123",
		Title:            "Test Video Title",
		Description:      "Test description",
		CategoryID:       28,
		Tags:             `["tech","ai"]`,
		DurationSeconds:  300,
		DefaultAudioLang: "en",
		PublishedAt:      now.Add(-2 * time.Hour),
		DiscoveredAt:     now,
		TrackedFromBirth: true,
	}
	if err := db.InsertVideo(v); err != nil {
		t.Fatalf("InsertVideo failed: %v", err)
	}

	exists, err := db.VideoExists("vid123")
	if err != nil || !exists {
		t.Errorf("Expected video to exist, got %v (err: %v)", exists, err)
	}

	// 3. Task management
	task := VideoTask{
		VideoID:           "vid123",
		PublishedAt:       v.PublishedAt,
		CurrentCheckpoint: 0,
		NextDueAt:         now.Add(-5 * time.Minute), // Due now
		CommentsHarvested: false,
		IsSealed:          false,
	}
	if err := db.InsertVideoTask(task); err != nil {
		t.Fatalf("InsertVideoTask failed: %v", err)
	}

	dueTasks, err := db.GetDueVideoTasks(now, 10)
	if err != nil || len(dueTasks) != 1 {
		t.Fatalf("Expected 1 due task, got %d (err: %v)", len(dueTasks), err)
	}

	// 4. Observation insertion
	obs := []Observation{
		{
			VideoID:               "vid123",
			CheckpointTargetHours: 1.0,
			ActualElapsedHours:    1.02,
			ObservedAt:            now,
			ViewCount:             5000,
			LikeCount:             250,
			CommentCount:          30,
			IsTrending:            false,
		},
	}
	if err := db.InsertObservations(obs); err != nil {
		t.Fatalf("InsertObservations failed: %v", err)
	}

	// 5. Comments insertion
	comments := []Comment{
		{
			CommentID:      "comm1",
			VideoID:        "vid123",
			AuthorChannel:  "UCauthor",
			Text:           "Great video!",
			PublishedAt:    now.Add(-1 * time.Hour),
			ElapsedMinutes: 60.0,
			LikeCount:      5,
			ReplyCount:     0,
		},
	}
	if err := db.InsertComments(comments); err != nil {
		t.Fatalf("InsertComments failed: %v", err)
	}

	// 6. Seal task and export query
	if err := db.UpdateTaskCheckpoint("vid123", 11, now, true); err != nil {
		t.Fatalf("UpdateTaskCheckpoint failed: %v", err)
	}

	sealedIDs, err := db.GetSealedUnexportedVideoIDs(10)
	if err != nil || len(sealedIDs) != 1 || sealedIDs[0] != "vid123" {
		t.Fatalf("Expected sealed video ID vid123, got %v (err: %v)", sealedIDs, err)
	}

	if err := db.MarkVideosExported(sealedIDs); err != nil {
		t.Fatalf("MarkVideosExported failed: %v", err)
	}

	unexportedAfter, _ := db.GetSealedUnexportedVideoIDs(10)
	if len(unexportedAfter) != 0 {
		t.Errorf("Expected 0 unexported videos after mark, got %d", len(unexportedAfter))
	}
}
