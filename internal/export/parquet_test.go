package export

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kryz73/collector/internal/database"
)

func TestParquetExport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "export_test_*")
	if err != nil {
		t.Fatalf("Failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed opening test DB: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	_ = db.InsertChannel(database.Channel{
		ChannelID: "UCtest", ChannelTitle: "Test", CategoryID: 28, Tier: "seed", AddedAt: now, IsActive: true,
	})
	_ = db.InsertVideo(database.Video{
		VideoID: "vid_exp", ChannelID: "UCtest", Title: "Export Test", CategoryID: 28, PublishedAt: now, DiscoveredAt: now, TrackedFromBirth: true,
	})
	_ = db.InsertVideoTask(database.VideoTask{
		VideoID: "vid_exp", PublishedAt: now, CurrentCheckpoint: 11, NextDueAt: now, IsSealed: true, SealedAt: &now,
	})
	_ = db.InsertObservations([]database.Observation{
		{VideoID: "vid_exp", CheckpointTargetHours: 1.0, ActualElapsedHours: 1.0, ObservedAt: now, ViewCount: 1000},
	})
	_ = db.InsertComments([]database.Comment{
		{CommentID: "c1", VideoID: "vid_exp", Text: "Hello", PublishedAt: now, ElapsedMinutes: 10},
	})

	exportDir := filepath.Join(tmpDir, "parquet_out")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	exporter := NewExporter(db, exportDir, logger)

	exported, err := exporter.ExportSealedVideos(10)
	if err != nil {
		t.Fatalf("ExportSealedVideos failed: %v", err)
	}
	if exported != 1 {
		t.Errorf("Expected 1 exported video, got %d", exported)
	}

	// Verify files were generated in date partition directory
	datePart := filepath.Join(exportDir, fmtDatePartition(now))
	files, err := os.ReadDir(datePart)
	if err != nil {
		t.Fatalf("Failed reading partition dir: %v", err)
	}

	if len(files) < 3 {
		t.Errorf("Expected at least 3 parquet files (videos, observations, comments), got %d", len(files))
	}
}

func fmtDatePartition(t time.Time) string {
	return "partition_date=" + t.Format("2006-01-02")
}
