#!/usr/bin/env python3
"""
Immediately export current live data from data/harvester.db to Snappy Parquet files in data/export/
without waiting for videos to finish their full 60-hour lifecycle.
"""

import os
import sqlite3
import pandas as pd
from datetime import datetime

def main():
    db_path = "data/harvester.db"
    if not os.path.exists(db_path):
        print(f"Error: {db_path} not found.")
        return

    now = datetime.utcnow()
    date_part = now.strftime("%Y-%m-%d")
    ts_suffix = now.strftime("%Y%m%d_%H%M%S")
    out_dir = os.path.join("data", "export", f"partition_date={date_part}")
    os.makedirs(out_dir, exist_ok=True)

    conn = sqlite3.connect(db_path)

    tables = ["channels", "videos", "observations", "comments", "trending_events"]
    print(f"Exporting live snapshot from {db_path} to {out_dir}/...")

    for tbl in tables:
        df = pd.read_sql(f"SELECT * FROM {tbl}", conn)
        out_file = os.path.join(out_dir, f"{tbl}_{ts_suffix}.parquet")
        df.to_parquet(out_file, compression="snappy", index=False)
        print(f"  [✓] {tbl:16}: {len(df):6} rows -> {out_file}")

    conn.close()
    print("Snapshot export complete!")

if __name__ == "__main__":
    main()
