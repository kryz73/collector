# YouTube Data Harvester (`harvest`)

A high-performance, standalone Go data ingestion daemon engineered to run 24/7 on AWS EC2, continuously harvesting temporally granular video engagement trajectories, dual-stage audience comments, and multi-region trending outcomes while strictly optimizing a **single YouTube Data API v3 key budget of 10,000 quota units/day**.

---

## 📑 Table of Contents
1. [Core Architectural Objectives](#-core-architectural-objectives)
2. [End-to-End System Architecture](#-end-to-end-system-architecture)
3. [Ingestion Loops & Scheduling](#-ingestion-loops--scheduling)
4. [Quota Economics: The 10,000 Units/Day Budget](#-quota-economics-the-10000-unitsday-budget)
5. [Multi-Region & Category Expansion](#-multi-region--category-expansion)
6. [Timezone & Circadian Handling](#-timezone--circadian-handling)
7. [Database Schema & Parquet Lake](#-database-schema--parquet-lake)
8. [Project Structure](#-project-structure)
9. [Deployment & Operations (AWS EC2)](#-deployment--operations-aws-ec2)
10. [CLI Flags & On-Demand Tools](#-cli-flags--on-demand-tools)

---

## 🎯 Core Architectural Objectives

1. **Zero-Quota Discovery**: Polls thousands of creator RSS/Atom feeds (`/feeds/videos.xml?channel_id=...`) over standard HTTP, consuming **0 Google Cloud quota units** to discover newly published videos within minutes of upload.
2. **Batch-Optimized Snapshots (50 IDs/call)**: Groups due checkpoint snapshots into 50-ID batches, achieving an efficiency of **0.02 quota units per observation snapshot**.
3. **60-Hour Non-Uniform State Machine**: Tracks every discovered video across 13 non-uniform checkpoints:
   $$t \in [0.5\text{h}, 1.0\text{h}, 1.5\text{h}, 2.0\text{h}, 3.0\text{h}, 4.0\text{h}, 5.0\text{h}, 6.0\text{h}, 12.0\text{h}, 24.0\text{h}, 36.0\text{h}, 48.0\text{h}, 60.0\text{h}]$$
   - Dense hourly & half-hourly sampling during early viral ignition ($t \le 6\text{h}$) minimizes finite-difference truncation error for velocity and acceleration estimation.
   - Sparse 12-hour sampling ($t > 6\text{h}$) captures asymptotic decay and circadian cycles without wasting quota.
4. **Dual-Stage Comment Harvesting (Sentiment Velocity)**:
   - **Stage 1 ($t = 1.0\text{h}$)**: Harvests up to 5 pages (~500 comments) capturing immediate audience excitement.
   - **Stage 2 ($t = 6.0\text{h}$)**: Harvests up to 15 pages (~1,500 comments) capturing discussion maturation.
   - Unlocks temporal sentiment drift: $\Delta S = S_{6h} - S_{1h}$.
5. **Multi-Region & Vertical Trending**: Scans 4 native English markets (`US`, `CA`, `GB`, `AU`) across 4 categories (General `0`, Music `10`, Gaming `20`, Entertainment `24`).
6. **Pure UTC Storage**: Eliminates timezone offset bugs by enforcing 100% UTC timestamps across SQLite and Parquet.

---

## 📐 End-to-End System Architecture

```text
                               YOUTUBE DATA HARVESTER (Go Daemon)
                                               │
        ┌────────────────────────┬─────────────┴─────────────┬────────────────────────┐
        ▼                        ▼                           ▼                        ▼
 1. Discovery Loop        2. Snapshot Loop            3. Trending Loop         4. Export Loop
 (Every 60m - 0 Quota)    (Every 5m - Batched)        (Every 1h - 4 Regions)   (Every 6h - Parquet)
 • 3,500+ RSS feeds       • 13 Checkpoints (0.5–60h)  • US, CA, GB, AU         • Sealed 60h records
 • Filters age <= 90m     • Batches 50 IDs / call     • General, Gaming,       • On-demand CLI dump
 • Queues in video_tasks  • Stage 1 & 2 Comments        Music, Entertainment   • Snappy Parquet files
                          • Stores in SQLite (WAL)    • Auto-expands creators  • Feeds Python ML models
```

---

## ⏱ Ingestion Loops & Scheduling

| Loop | Interval | Mechanism | Quota Cost | Purpose |
|---|---|---|---|---|
| **Discovery Loop** | **Every 60 min** | 30-worker HTTP pool reading Atom feeds | **0 units** | Scans 3,500 creator feeds for new uploads published within the last 90 minutes. |
| **Snapshot Loop** | **Every 5 min** | `SELECT ... WHERE is_sealed=0 AND next_due_at <= now` | **~50 units/day** | Fetches view/like/comment counts in 50-ID batches; triggers Stage 1 (1h) and Stage 2 (6h) comment harvesting. |
| **Trending Loop** | **Every 60 min** | `videos.list(chart=mostPopular, regionCode, categoryId)` | **~384 units/day** | Scans US, CA, GB, AU leaderboards across 4 categories (16 calls/hr); auto-registers newly discovered trending creators. |
| **Export Loop** | **Every 6 hours** | Generic Parquet writer with Snappy compression | **0 units** | Flushes sealed 60-hour video trajectories into analytical Parquet partitions. |

---

## 📊 Quota Economics: The 10,000 Units/Day Budget

The service contains a built-in, thread-safe **Quota Governor** with an automatic midnight Pacific Time (07:00 UTC) reset and priority tiers (Critical, Normal, Low).

| Ingestion Task | Daily Volume | Cost per Call | Total Daily Quota |
|---|---|---|---|
| **RSS Channel Discovery** (3,500 creators) | 24 cycles $\times$ 3,500 feeds | 0 (HTTP GET) | **0 units** |
| **New Video Metadata Ingestion** | ~350 videos / 50 per batch | 1 unit | **~7 units** |
| **Observation Snapshots** (13 checkpoints) | ~800 active videos / 50 per batch | 1 unit | **~52 units** |
| **Multi-Region & Category Trending** | 4 regions $\times$ 4 categories $\times$ 24 hours | 1 unit | **384 units** |
| **Stage 1 Comments ($t = 1.0\text{h}$)** | ~250 videos $\times$ 4 pages avg | 1 unit / page | **~1,000 units** |
| **Stage 2 Comments ($t = 6.0\text{h}$)** | ~250 videos $\times$ 15 pages avg | 1 unit / page | **~3,750 units** |
| **Trending Viral Video Comment Ingestion** | ~50 untracked videos $\times$ 10 pages | 1 unit / page | **~500 units** |
| **Expected Daily Burn** | | | **~5,693 units** |
| **Peak Day Utilization (High Upload Days)** | | | **~7,500–8,000 units** |
| **Safety Reserve Buffer** | Reserved for retries/spikes | | **~2,000 units (20%)** |

---

## 🌍 Multi-Region & Category Expansion

To eliminate single-region bias without introducing multi-language translation complexities, the harvester targets **4 primary native English-speaking markets**:

- **`US` (United States)**: Primary global media market.
- **`CA` (Canada)**: Overlapping North American media culture.
- **`GB` (Great Britain)**: Primary European English market; leads US timezones by 5–8 hours.
- **`AU` (Australia)**: Primary Oceanic English market; leads US timezones by 14–17 hours.

### Monitored Categories:
- **`0`**: All / General Trending (YouTube Homepage)
- **`10`**: Music
- **`20`**: Gaming
- **`24`**: Entertainment

---

## ⏰ Timezone & Circadian Handling

### The Problem with Confounded Heuristics
A common pitfall is attempting to infer audience location from raw comment arrival peaks. **Comment arrival is heavily dominated by publication recency ($t - t_{\text{pub}}$)**: viewers comment immediately upon receiving upload notifications. A US creator uploading at 2:00 PM EST (19:00 UTC) experiences peak comments at 19:00–21:00 UTC *not because the audience is in London*, but because the video was published at that moment.

### How We Handle Timezones Grounded in Real Data:
1. **100% Invariant Physical Elapsed Time ($\Delta t$)**:
   Every checkpoint and derivative ($v = \frac{dV}{dt}$, $a = \frac{d^2V}{dt^2}$) is computed as:
   $$\Delta t = \text{observed\_at (UTC)} - \text{published\_at (UTC)}$$
   Physical elapsed time is identical everywhere on Earth.
2. **Publisher Local Time Anchor**:
   Using the creator's country (from the seed registry / channel metadata), the pipeline maps uploads to canonical timezones:
   - `US` / `CA` $\to$ `America/New_York` (ET)
   - `GB` $\to$ `Europe/London` (GMT/BST)
   - `AU` $\to$ `Australia/Sydney` (AEST/AEDT)
3. **Cyclical Periodic Encodings ($\sin$ and $\cos$)**:
   Local publication hour $h \in [0, 24)$ is transformed into continuous periodic features:
   $$h_{\sin} = \sin\left(\frac{2\pi h}{24}\right), \quad h_{\cos} = \cos\left(\frac{2\pi h}{24}\right)$$
4. **Empirical Geographic Virality Cascades**:
   Instead of guessing where the audience is, we directly observe ground truth in `trending_events`:
   - Does the video chart in `GB` and `AU` first before hitting `US`?
   - Multi-region trending presence (`cross_region_count`) serves as a powerful predictive feature for North American virality.

---

## 🗄 Database Schema & Parquet Lake

### Relational Schema (SQLite with WAL Mode)
- **`channels`**: Channel ID, title, category, seed tier, last upload timestamp, country, active status.
- **`videos`**: Video ID, channel ID, title, description, category, tags, duration, definition (HD/SD), caption flag, COPPA kids flag, live broadcast status, Wikipedia topic categories, published_at (UTC).
- **`video_tasks`**: State machine tracking `current_checkpoint`, `next_due_at`, `comments_stage` (0=none, 1=1h, 2=6h), `is_sealed`.
- **`observations`**: Checkpoint target hours, actual elapsed hours, observed_at (UTC), view_count, like_count, comment_count, is_trending.
- **`comments`**: Comment ID, video ID, author channel/name, unescaped raw text (`TextOriginal`), published_at (UTC), elapsed_minutes, like_count, reply_count.
- **`trending_events`**: Video ID, region_code (`US`, `CA`, `GB`, `AU`), category_id (`0`, `10`, `20`, `24`), trending_rank, captured_at (UTC).

### Parquet Lake Structure
When records are exported (either automatically every 6 hours or via `./bin/harvester -export-now`), snappy-compressed Parquet files are written to:
```text
data/export/partition_date=YYYY-MM-DD/
├── videos_all_YYYYMMDD_HHMMSS.parquet
├── observations_all_YYYYMMDD_HHMMSS.parquet
├── comments_all_YYYYMMDD_HHMMSS.parquet
└── trending_events_all_YYYYMMDD_HHMMSS.parquet
```

---

## 🗂 Project Structure

```text
harvest/
├── cmd/
│   └── harvester/
│       └── main.go                 # Daemon entrypoint, flag parsing, signal handling
├── internal/
│   ├── config/
│   │   └── config.go               # YAML configuration loader & .env integration
│   ├── database/
│   │   ├── db.go                   # SQLite connection with WAL mode & pragmas
│   │   ├── migrations.go           # Relational schema initialization & dynamic migrations
│   │   ├── models.go               # Struct definitions with DB and Parquet tags
│   │   └── queries.go              # Complete CRUD & analytical export queries
│   ├── discovery/
│   │   └── rss.go                  # Atom/RSS feed parser with worker pool concurrency
│   ├── youtube/
│   │   ├── client.go               # YouTube Data API v3 wrapper (batching, retries, categories)
│   │   └── duration.go             # ISO 8601 duration parser
│   ├── quota/
│   │   └── governor.go             # Daily quota tracker, throttling, midnight PT reset
│   ├── scheduler/
│   │   └── scheduler.go            # 4 concurrent loops (Discovery, Snapshots, Trending, Export)
│   ├── export/
│   │   └── parquet.go              # Snappy-compressed Parquet exporter
│   └── registry/
│       └── channels.go             # Seed channel manager & LRU auto-expansion
├── configs/
│   ├── harvester.yaml              # Master runtime configuration
│   └── seed_channels.json          # 3,500 stratified creators (Tier 1: 500, Tier 2: 1500, Tier 3: 1500)
├── scripts/
│   ├── extract_seed_channels.py    # Extracts active US/CA creators from Kaggle datasets
│   └── export_snapshot_to_parquet.py # Python alternative for Parquet export
├── docs/
│   └── TIMEZONE_AND_MULTIREGION.md # Mathematical guide for timezones & circadian features
├── deployments/
│   ├── viral-harvester.service     # systemd unit configuration for 24/7 background execution
│   └── setup.sh                    # Automated EC2 bootstrap script (Go, swap, systemd)
├── bin/
│   └── harvester                   # Compiled standalone static binary
├── Makefile                        # Build, test, cross-compilation targets
├── Dockerfile                      # Multi-stage scratch build (~20 MB container)
├── go.mod                          # Go module dependencies
└── README.md                       # Master project documentation
```

---

## 🚀 Deployment & Operations (AWS EC2)

### 1. Recommended EC2 Instance
- **Instance Type**: `t4g.micro` (AWS Graviton2 ARM64, 2 vCPU, 1 GB RAM)
- **OS**: Ubuntu 24.04 LTS (ARM64)
- **Storage**: 20 GiB gp3 (3,000 IOPS baseline)
- **Monthly Cost**: **~\$7.70 / month** (Runs 13+ months on \$100 AWS credits)

### 2. Initial Setup on Server
```bash
# 1. Clone repository
git clone https://github.com/kryz73/harvest.git
cd harvest

# 2. Run bootstrap script (auto-creates 2GB swap and builds binary)
./deployments/setup.sh

# 3. Configure API key
nano .env
# Set: YOUTUBE_API_KEY="AIzaSy..."

# 4. Enable and start 24/7 background service
sudo systemctl enable --now viral-harvester
```

### 3. Monitoring & Inspection Commands
```bash
# Check service status
sudo systemctl status viral-harvester

# View live log stream
journalctl -u viral-harvester -f

# Check database counts
python3 -c "import sqlite3; conn=sqlite3.connect('data/harvester.db'); print([(t, conn.cursor().execute(f'SELECT count(*) FROM {t}').fetchone()[0]) for t in ['channels','videos','observations','comments','trending_events']])"
```

---

## 🛠 CLI Flags & On-Demand Tools

The compiled binary supports operational CLI flags:

### 1. On-Demand Parquet Export (No waiting 60 hours):
```bash
./bin/harvester -export-now
```
Reads the live SQLite database and dumps all current videos, observations, comments, and trending events into Parquet files in `data/export/partition_date=YYYY-MM-DD/` in under 20 milliseconds.

### 2. Custom Configuration File:
```bash
./bin/harvester -config /path/to/custom_harvester.yaml
```

### 3. Regenerating Seed Channels from Kaggle:
```bash
python3 scripts/extract_seed_channels.py
```
Re-extracts and stratifies active creators from `data/*trending_data.csv.zip` into `configs/seed_channels.json`.