# YouTube Data Harvester (`collector`)

A high-performance, standalone Go data ingestion daemon designed to run 24/7 on AWS EC2, continuously harvesting temporally precise video engagement metrics, early audience comments, and regional trending outcomes while staying strictly within a **single YouTube API key limit of 10,000 quota units/day**.

---

## 🎯 Key Capabilities

- **0-Quota Channel Discovery**: Uses YouTube public RSS/Atom feeds (`/feeds/videos.xml?channel_id=...`) to monitor 1,500+ channels without burning a single API quota unit.
- **Batched Observation Tracking (50 IDs/call)**: Batches snapshots to achieve the theoretical maximum efficiency of **0.02 quota units per video snapshot**.
- **60-Hour Lifecycle State Machine**: Tracks every discovered video across 11 discrete temporal checkpoints ($1\text{h}, 2\text{h}, 3\text{h}, 4\text{h}, 5\text{h}, 6\text{h}, 12\text{h}, 24\text{h}, 36\text{h}, 48\text{h}, 60\text{h}$) before permanently sealing the record.
- **Strict Anti-Leakage Early Comments**: Harvests comments at the 6-hour observation cutoff, filtering strictly for `comment.published_at <= video.published_at + 6 hours`.
- **Opportunistic Trending Ingestion**: Ingests untracked trending videos in the US and Canada and dynamically adds new viral creators to the seed registry with LRU-based capacity caps.
- **Embedded SQLite (WAL Mode) + Parquet Export**: Pure Go SQLite storage (`modernc.org/sqlite` — zero CGO) periodically exported into snappy-compressed Apache Parquet datasets ready for Polars/Pandas ML training.
- **Built-in Quota Governor**: Thread-safe daily quota tracking with automatic midnight Pacific Time resets and priority-based throttling.

---

## 📐 Architecture Overview

```text
                               YOUTUBE DATA HARVESTER (Go Daemon)
                                                │
         ┌────────────────────────┬─────────────┴─────────────┬────────────────────────┐
         ▼                        ▼                           ▼                        ▼
  1. Discovery Loop        2. Snapshot Loop            3. Trending Loop         4. Export Loop
  (Every 90m - 0 Quota)    (Every 5m - Batched)        (Every 2h - US & CA)     (Every 6h - Parquet)
  • Public RSS/Atom feeds  • 11 checkpoints (1h–60h)   • chart=mostPopular      • Dumps sealed records
  • Filters videos <= 90m  • Batches 50 IDs / call     • Logs peak rank         • Snappy Parquet files
  • Queues in video_tasks  • Harvests comments at 6h   • Auto-expands channels  • Consumed by ML model
```

---

## 📊 Quota Economics: The 10,000 Units/Day Budget

| Ingestion Task | Frequency | Daily API Calls | Daily Quota Used | Share of 10,000 Quota |
|---|---|---|---|---|
| **Channel Discovery** (1,500 channels) | Every 90m via RSS feeds | 0 (HTTP GET) | **0 units** | 0.0% |
| **New Video Metadata** | ~200 videos/day (batched by 50) | 4 | **4 units** | 0.04% |
| **60h Snapshots** (11 checkpoints) | ~500 active videos (batched by 50) | 44 | **44 units** | 0.44% |
| **US & CA Trending Checks** | Every 2 hours | 24 | **24 units** | 0.24% |
| **Deep Early Comments** ($t \le 6\text{h}$) | ~200 videos $\times$ 3–5 pages | ~800 | **~800 units** | 8.00% |
| **Untracked Trending Comments** | ~30 videos $\times$ 3 pages | ~90 | **~90 units** | 0.90% |
| **Safety Reserve / Retries** | Dynamic backoff | ~100 | **~100 units** | 1.00% |
| **TOTAL DAILY USAGE** | | | **~1,062 units** | **~10.6%** |

> **Safety Margin**: Consumes only **~10.6% of your daily quota**, leaving an 89% buffer (8,938 units) untouched!

---

## 🗂 Project Structure

```text
collector/
├── cmd/
│   └── harvester/
│       └── main.go                 # Daemon entrypoint, signal handling, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go               # YAML configuration loader & env var overrides
│   ├── database/
│   │   ├── db.go                   # SQLite connection with WAL mode and pragmas
│   │   ├── migrations.go           # Relational schema initialization
│   │   ├── models.go               # Struct definitions with DB and Parquet tags
│   │   └── queries.go              # Complete transactional queries
│   ├── discovery/
│   │   └── rss.go                  # Atom/RSS feed parser with worker pool concurrency
│   ├── youtube/
│   │   ├── client.go               # YouTube Data API v3 wrapper (batching, retries)
│   │   └── duration.go             # ISO 8601 duration parser
│   ├── quota/
│   │   └── governor.go             # Daily quota tracker, throttling, midnight PT reset
│   ├── scheduler/
│   │   └── scheduler.go            # 4 concurrent goroutine loops
│   ├── export/
│   │   └── parquet.go              # Snappy-compressed Parquet exporter
│   └── registry/
│       └── channels.go             # Seed channel manager & LRU auto-expansion
├── configs/
│   ├── harvester.yaml              # Default configuration parameters
│   └── seed_channels.json          # Seed creator registry
├── deployments/
│   ├── viral-harvester.service     # systemd daemon configuration for EC2
│   └── setup.sh                    # Automated EC2 bootstrap script
├── bin/
│   └── harvester                   # Compiled static binary
├── Makefile                        # Build, test, cross-compilation targets
├── Dockerfile                      # Multi-stage scratch build (~20 MB)
├── go.mod                          # Go module definition
└── README.md                       # Project documentation
```

---

## 🚀 Quick Start (Local)

### 1. Prerequisites
- Go 1.22+ (tested with Go 1.27)
- A YouTube Data API v3 Key

### 2. Build the Harvester
```bash
make build
```
This generates a single static binary in `bin/harvester`.

### 3. Run Automated Tests
```bash
make test
```

### 4. Run the Daemon
```bash
export YOUTUBE_API_KEY="your-google-api-key"
./bin/harvester -config configs/harvester.yaml
```

---

## ☁️ AWS EC2 Deployment

### 1. Launch EC2 Instance
- **Instance Type**: `t4g.small` (ARM Graviton2, ~\$12/month) or `t3.small` (x86).
- **OS**: Ubuntu 24.04 LTS.
- **Storage**: 30 GiB gp3 EBS.

### 2. Bootstrap the Server
SSH into your instance and run:
```bash
git clone git@github.com:kryz73/collector.git
cd collector
./deployments/setup.sh
```

### 3. Configure API Key & Start
Edit `.env`:
```bash
nano .env
# Set YOUTUBE_API_KEY="your-key"
```

Start the background service:
```bash
sudo systemctl enable --now viral-harvester
```

Check live status and logs:
```bash
sudo systemctl status viral-harvester
journalctl -u viral-harvester -f
```

---

## 📦 Parquet Export Schema (Downstream ML Consumption)

Every 6 hours, completed video records are exported to `data/export/partition_date=YYYY-MM-DD/`:
- `videos_*.parquet`: Video metadata (`video_id`, `channel_id`, `title`, `duration_seconds`, `category_id`, `published_at`).
- `observations_*.parquet`: Complete 11-point time series (`checkpoint_target_hours`, `actual_elapsed_hours`, `view_count`, `like_count`, `comment_count`, `is_trending`).
- `comments_*.parquet`: Early audience reaction text stream (`comment_id`, `text`, `published_at`, `elapsed_minutes`, `like_count`).
- `trending_events_*.parquet`: Regional trending rankings (`region_code`, `trending_rank`, `captured_at`).

Load directly into Polars or DuckDB in your Python ML pipeline:
```python
import polars as pl
df_obs = pl.read_parquet("collector/data/export/**/*.parquet")
```

---

## 📄 License
MIT License.