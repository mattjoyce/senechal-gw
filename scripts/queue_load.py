#!/usr/bin/env python3
import requests
import argparse
import time

def main():
    parser = argparse.ArgumentParser(description="Queue many jobs via Ductile API.")
    parser.add_argument("--url", default="http://localhost:18080", help="Base URL of Ductile API")
    parser.add_argument("--token", default="bench-token-123", help="API Token")
    parser.add_argument("--plugin", required=True, help="Plugin name")
    parser.add_argument("--command", required=True, help="Command name")
    parser.add_argument("--count", type=int, default=10, help="Number of jobs to queue")
    parser.add_argument("--payload", default="{}", help="JSON payload for the jobs")

    args = parser.parse_args()

    headers = {
        "Authorization": f"Bearer {args.token}",
        "Content-Type": "application/json"
    }

    import json
    payload_json = json.loads(args.payload)

    endpoint = f"{args.url}/plugin/{args.plugin}/{args.command}"
    
    print(f"Queuing {args.count} jobs to {endpoint}...")
    
    success_count = 0
    for i in range(args.count):
        try:
            response = requests.post(endpoint, headers=headers, json={"payload": payload_json})
            if response.status_code == 202:
                success_count += 1
            else:
                print(f"Failed to queue job {i+1}: {response.status_code} {response.text}")
        except Exception as e:
            print(f"Error queuing job {i+1}: {e}")
            
    print(f"Successfully queued {success_count}/{args.count} jobs.")

if __name__ == "__main__":
    main()
