package export

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kryz73/collector/internal/database"
	"github.com/parquet-go/parquet-go"
)

type Exporter struct {
	db        *database.DB
	outputDir string
	logger    *slog.Logger
}

func NewExporter(db *database.DB, outputDir string, logger *slog.Logger) *Exporter {
	return &Exporter{
		db:        db,
		outputDir: outputDir,
		logger:    logger,
	}
}

// ExportSealedVideos exports batches of sealed videos and their time-series data to Snappy Parquet files.
func (e *Exporter) ExportSealedVideos(batchSize int) (int, error) {
	videoIDs, err := e.db.GetSealedUnexportedVideoIDs(batchSize)
	if err != nil {
		return 0, fmt.Errorf("getting unexported sealed videos: %w", err)
	}

	if len(videoIDs) == 0 {
		return 0, nil
	}

	if err := os.MkdirAll(e.outputDir, 0755); err != nil {
		return 0, fmt.Errorf("creating export directory: %w", err)
	}

	now := time.Now().UTC()
	datePartition := now.Format("2006-01-02")
	partDir := filepath.Join(e.outputDir, fmt.Sprintf("partition_date=%s", datePartition))
	if err := os.MkdirAll(partDir, 0755); err != nil {
		return 0, fmt.Errorf("creating date partition dir %s: %w", partDir, err)
	}

	timestampSuffix := now.Format("20060102_150405")

	// 1. Export Videos
	videos, err := e.db.GetVideosByIDs(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("getting videos for export: %w", err)
	}
	if len(videos) > 0 {
		videoFile := filepath.Join(partDir, fmt.Sprintf("videos_%s.parquet", timestampSuffix))
		if err := writeParquet(videoFile, videos); err != nil {
			return 0, fmt.Errorf("writing videos parquet: %w", err)
		}
	}

	// 2. Export Observations
	obs, err := e.db.GetObservationsByVideoIDs(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("getting observations for export: %w", err)
	}
	if len(obs) > 0 {
		obsFile := filepath.Join(partDir, fmt.Sprintf("observations_%s.parquet", timestampSuffix))
		if err := writeParquet(obsFile, obs); err != nil {
			return 0, fmt.Errorf("writing observations parquet: %w", err)
		}
	}

	// 3. Export Comments
	comments, err := e.db.GetCommentsByVideoIDs(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("getting comments for export: %w", err)
	}
	if len(comments) > 0 {
		commentFile := filepath.Join(partDir, fmt.Sprintf("comments_%s.parquet", timestampSuffix))
		if err := writeParquet(commentFile, comments); err != nil {
			return 0, fmt.Errorf("writing comments parquet: %w", err)
		}
	}

	// 4. Export Trending Events
	events, err := e.db.GetTrendingEventsByVideoIDs(videoIDs)
	if err != nil {
		return 0, fmt.Errorf("getting trending events for export: %w", err)
	}
	if len(events) > 0 {
		eventsFile := filepath.Join(partDir, fmt.Sprintf("trending_events_%s.parquet", timestampSuffix))
		if err := writeParquet(eventsFile, events); err != nil {
			return 0, fmt.Errorf("writing trending events parquet: %w", err)
		}
	}

	// Mark exported in DB
	if err := e.db.MarkVideosExported(videoIDs); err != nil {
		return 0, fmt.Errorf("marking videos as exported: %w", err)
	}

	e.logger.Info("Sealed videos exported to Parquet",
		"count", len(videoIDs),
		"observations", len(obs),
		"comments", len(comments),
		"partition", datePartition,
	)

	return len(videoIDs), nil
}

func writeParquet[T any](path string, records []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[T](f, parquet.Compression(&parquet.Snappy))
	if _, err := writer.Write(records); err != nil {
		return err
	}
	return writer.Close()
}
