#!/usr/bin/env python3
import sqlite3
import uuid
import json
import argparse
import datetime
import os
import sys

def main():
    parser = argparse.ArgumentParser(description="Bulk enqueue jobs into Ductile SQLite database.")
    parser.add_argument("--db", default="./ductile.db", help="Path to ductile.db")
    parser.add_argument("--plugin", required=True, help="Plugin name")
    parser.add_argument("--command", required=True, help="Command name")
    parser.add_argument("--count", type=int, default=10, help="Number of jobs to enqueue")
    parser.add_argument("--payload", default="{}", help="JSON payload for the jobs")
    parser.add_argument("--submitted-by", default="bench", help="submitted_by value")

    args = parser.parse_args()

    if not os.path.exists(args.db):
        print(f"Error: Database not found at {args.db}")
        sys.exit(1)

    try:
        payload_json = json.loads(args.payload)
    except json.JSONDecodeError:
        print("Error: Invalid JSON payload")
        sys.exit(1)

    conn = sqlite3.connect(args.db)
    cursor = conn.cursor()

    now = datetime.datetime.now(datetime.timezone.utc).isoformat()

    print(f"Enqueuing {args.count} jobs for {args.plugin}:{args.command}...")

    for i in range(args.count):
        job_id = str(uuid.uuid4())
        cursor.execute("""
            INSERT INTO job_queue (
                id, plugin, command, payload, status, attempt, max_attempts, 
                submitted_by, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            job_id, args.plugin, args.command, json.dumps(payload_json), 
            "queued", 1, 3, args.submitted_by, now
        ))

    conn.commit()
    conn.close()
    print("Done.")

if __name__ == "__main__":
    main()
