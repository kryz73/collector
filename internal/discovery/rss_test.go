package discovery

import (
	"encoding/xml"
	"testing"
	"time"
)

const sampleFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
  <title>Sample Channel</title>
  <entry>
    <yt:videoId>dQw4w9WgXcQ</yt:videoId>
    <yt:channelId>UCuAXFkgsw1L7xaCfnd5JJOw</yt:channelId>
    <title>Rick Astley - Never Gonna Give You Up (Official Music Video)</title>
    <published>2026-10-04T12:00:00+00:00</published>
  </entry>
  <entry>
    <yt:videoId>9bZkp7q19f0</yt:videoId>
    <yt:channelId>UCuAXFkgsw1L7xaCfnd5JJOw</yt:channelId>
    <title>PSY - GANGNAM STYLE(강남스타일) M/V</title>
    <published>2026-10-04T11:30:00+00:00</published>
  </entry>
</feed>`

func TestParseAtomFeed(t *testing.T) {
	var feed atomFeed
	err := xml.Unmarshal([]byte(sampleFeedXML), &feed)
	if err != nil {
		t.Fatalf("Failed to unmarshal sample XML: %v", err)
	}

	if len(feed.Entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(feed.Entries))
	}

	e1 := feed.Entries[0]
	if e1.VideoID != "dQw4w9WgXcQ" {
		t.Errorf("Expected videoId dQw4w9WgXcQ, got %s", e1.VideoID)
	}
	if e1.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("Expected channelId UCuAXFkgsw1L7xaCfnd5JJOw, got %s", e1.ChannelID)
	}
	if e1.Title != "Rick Astley - Never Gonna Give You Up (Official Music Video)" {
		t.Errorf("Unexpected title: %s", e1.Title)
	}

	expectedPub := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if !e1.Published.Equal(expectedPub) {
		t.Errorf("Expected published time %v, got %v", expectedPub, e1.Published)
	}
}
