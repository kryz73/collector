#!/usr/bin/env python3
"""
Extract top 1,500 active US/CA trending channels from Kaggle historical trending dataset.
Outputs stratified configs/seed_channels.json.
"""

import os
import json
import zipfile
from collections import defaultdict, Counter
import pandas as pd

def main():
    data_dir = "data"
    us_zip = os.path.join(data_dir, "US_youtube_trending_data.csv.zip")
    ca_zip = os.path.join(data_dir, "CA_youtube_trending_data.csv.zip")

    if not os.path.exists(us_zip) and not os.path.exists(ca_zip):
        print("Error: Kaggle trending zip files not found in data/")
        return

    channel_stats = defaultdict(lambda: {
        'titles': Counter(),
        'categories': Counter(),
        'videos': set(),
        'latest_date': ''
    })

    for zip_path in [us_zip, ca_zip]:
        if not os.path.exists(zip_path):
            continue
        print(f"Reading {zip_path}...")
        with zipfile.ZipFile(zip_path) as z:
            fname = z.namelist()[0]
            with z.open(fname) as f:
                for chunk in pd.read_csv(f, usecols=['video_id', 'channelId', 'channelTitle', 'categoryId', 'trending_date'], chunksize=50000):
                    for _, row in chunk.iterrows():
                        cid = str(row['channelId']).strip()
                        if not cid or cid == 'nan' or len(cid) < 10:
                            continue
                        c = channel_stats[cid]
                        title = str(row['channelTitle']).strip()
                        if title and title != 'nan':
                            c['titles'][title] += 1
                        cat = int(row['categoryId']) if pd.notna(row['categoryId']) else 0
                        c['categories'][cat] += 1
                        c['videos'].add(str(row['video_id']))
                        dt = str(row['trending_date'])
                        if dt > c['latest_date']:
                            c['latest_date'] = dt

    print(f"Total unique channels discovered: {len(channel_stats)}")

    # Filter for active channels (trending appearance in 2023 or 2024)
    active_channels = []
    for cid, stats in channel_stats.items():
        if not stats['titles']:
            continue
        # Filter for recent activity
        if stats['latest_date'] >= '2023-01-01':
            title = stats['titles'].most_common(1)[0][0]
            category_id = stats['categories'].most_common(1)[0][0]
            video_count = len(stats['videos'])
            active_channels.append({
                'channel_id': cid,
                'name': title,
                'category_id': category_id,
                'video_count': video_count,
                'latest_date': stats['latest_date']
            })

    print(f"Active channels (2023-2024): {len(active_channels)}")

    # Sort by trending video count descending
    active_channels.sort(key=lambda x: x['video_count'], reverse=True)

    # Stratify into 3 tiers:
    # Tier 1: Top 500 channels (Heavy trenders)
    # Tier 2: Next 1,500 channels (Frequent trenders)
    # Tier 3: Next 1,500 channels (Breakout creators)
    tier_1 = active_channels[:500]
    tier_2 = active_channels[500:2000]
    tier_3 = active_channels[2000:3500]

    def clean_tier(ch_list):
        return [
            {
                'channel_id': c['channel_id'],
                'name': c['name'],
                'category_id': c['category_id']
            }
            for c in ch_list
        ]

    output_data = {
        "description": "Stratified Seed Creator Registry for YouTube Trending Harvester (Extracted from US & CA Historical Trending Data)",
        "tiers": {
            "tier_1_high_potential": {
                "target_subscriber_range": "1M - 10M+",
                "expected_trending_rate": 0.40,
                "count": len(tier_1),
                "channels": clean_tier(tier_1)
            },
            "tier_2_battleground": {
                "target_subscriber_range": "100K - 1M",
                "expected_trending_rate": 0.15,
                "count": len(tier_2),
                "channels": clean_tier(tier_2)
            },
            "tier_3_breakout": {
                "target_subscriber_range": "25K - 100K",
                "expected_trending_rate": 0.05,
                "count": len(tier_3),
                "channels": clean_tier(tier_3)
            }
        }
    }

    out_file = "configs/seed_channels.json"
    with open(out_file, "w", encoding="utf-8") as f:
        json.dump(output_data, f, indent=2, ensure_ascii=False)

    print(f"Successfully generated {out_file} with {len(tier_1) + len(tier_2) + len(tier_3)} channels!")
    print(f"  Tier 1: {len(tier_1)} channels")
    print(f"  Tier 2: {len(tier_2)} channels")
    print(f"  Tier 3: {len(tier_3)} channels")

if __name__ == "__main__":
    main()
