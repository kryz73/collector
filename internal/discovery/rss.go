package discovery

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kryz73/harvest/internal/database"
)

// Atom XML structures matching YouTube's public feeds/videos.xml format
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	VideoID   string    `xml:"videoId"`
	ChannelID string    `xml:"channelId"`
	Title     string    `xml:"title"`
	Published time.Time `xml:"published"`
}

type DiscoveredVideo struct {
	VideoID     string
	ChannelID   string
	Title       string
	PublishedAt time.Time
}

type Poller struct {
	httpClient  *http.Client
	concurrency int
	logger      *slog.Logger
}

func NewPoller(concurrency int, logger *slog.Logger) *Poller {
	return &Poller{
		httpClient: &http.Client{
			Timeout: 12 * time.Second,
		},
		concurrency: concurrency,
		logger:      logger,
	}
}

// PollChannel fetches and parses the RSS feed for a single YouTube channel.
func (p *Poller) PollChannel(ctx context.Context, channelID string) ([]DiscoveredVideo, error) {
	url := fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", channelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ResearchHarvester/1.0)")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching feed for %s: %w", channelID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed HTTP %d for %s", resp.StatusCode, channelID)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading feed body for %s: %w", channelID, err)
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("unmarshaling atom XML for %s: %w", channelID, err)
	}

	var results []DiscoveredVideo
	for _, e := range feed.Entries {
		if e.VideoID != "" {
			results = append(results, DiscoveredVideo{
				VideoID:     e.VideoID,
				ChannelID:   e.ChannelID,
				Title:       e.Title,
				PublishedAt: e.Published.UTC(),
			})
		}
	}

	return results, nil
}

// PollAllChannels polls all active channels concurrently, filtering videos by max age.
func (p *Poller) PollAllChannels(ctx context.Context, channels []database.Channel, maxAgeMinutes int) []DiscoveredVideo {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeMinutes) * time.Minute)
	chQueue := make(chan database.Channel, len(channels))
	for _, c := range channels {
		chQueue <- c
	}
	close(chQueue)

	var mu sync.Mutex
	var discovered []DiscoveredVideo
	var wg sync.WaitGroup

	numWorkers := p.concurrency
	if numWorkers > len(channels) && len(channels) > 0 {
		numWorkers = len(channels)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range chQueue {
				select {
				case <-ctx.Done():
					return
				default:
				}

				videos, err := p.PollChannel(ctx, c.ChannelID)
				if err != nil {
					p.logger.Debug("RSS poll failed for channel", "channel_id", c.ChannelID, "error", err)
					continue
				}

				for _, v := range videos {
					if v.PublishedAt.After(cutoff) {
						mu.Lock()
						discovered = append(discovered, v)
						mu.Unlock()
					}
				}
			}
		}()
	}

	wg.Wait()
	return discovered
}
