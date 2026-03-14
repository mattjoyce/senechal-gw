#!/usr/bin/env python3
import sys
import json
import time
import hashlib
import os
import urllib.request
import math

def emit_event(name, data):
    print(json.dumps({"event": {"name": name, "data": data}}))

def do_cpu(duration):
    start = time.time()
    count = 0
    while time.time() - start < duration:
        # Some heavy math that won't overflow easily
        for i in range(1000):
            _ = math.sin(i) * math.cos(i) * math.tan(i)
        count += 1
    return time.time() - start

def do_io(size_mb, workspace_dir=None):
    # Prefer workspace dir to avoid writing into read-only plugin mounts.
    base_dir = workspace_dir if workspace_dir and os.path.isdir(workspace_dir) else "/tmp"
    path = os.path.join(base_dir, "stress_test_data.tmp")
    chunk = b"X" * 1024 * 1024  # 1MB
    start = time.time()

    h = hashlib.sha256()
    with open(path, "wb") as f:
        for _ in range(size_mb):
            f.write(chunk)
            h.update(chunk)

    write_duration = time.time() - start
    digest = h.hexdigest()

    if os.path.exists(path):
        os.remove(path)

    return digest, write_duration

def do_net(url):
    try:
        with urllib.request.urlopen(url, timeout=10) as response:
            data = response.read()
            return response.getcode(), len(data)
    except Exception as e:
        return 0, str(e)

def main():
    raw_input = sys.stdin.read()

    input_data = {}
    if raw_input:
        try:
            input_data = json.loads(raw_input)
        except json.JSONDecodeError:
            print(json.dumps({"status": "error", "error": "invalid request JSON"}))
            sys.exit(1)

    command = input_data.get("command", "health")
    payload = input_data.get("payload", {})
    state = input_data.get("state", {})
    workspace_dir = input_data.get("workspace_dir")

    result_data = {"status": "ok"}
    events = []
    state_updates = {}

    try:
        if command == "cpu":
            duration = payload.get("duration_seconds", 5)
            actual = do_cpu(duration)
            result_data = {"result": "calculations complete", "actual_duration": actual}

        elif command == "io":
            size = payload.get("size_mb", 10)
            # Safety cap
            if size > 1000:
                size = 1000
            digest, duration = do_io(size, workspace_dir)
            result_data = {
                "hash": digest,
                "write_speed_mbps": size / duration if duration > 0 else 0,
            }

        elif command == "net":
            url = payload.get("url", "https://www.google.com")
            code, size = do_net(url)
            result_data = {"status_code": code, "bytes_received": size if isinstance(size, int) else 0}
            if isinstance(size, str):
                result_data["error"] = size

        elif command == "state":
            iterations = payload.get("iterations", 10)
            current_count = state.get("count", 0)

            for i in range(iterations):
                current_count += 1
                events.append({
                    "type": "stress.state_increment",
                    "payload": {"step": i, "total": current_count},
                })

            result_data = {"final_count": current_count}
            state_updates = {"count": current_count}

        elif command == "health":
            result_data = {"status": "ok", "version": "0.1.0"}

        else:
            print(json.dumps({
                "status": "error",
                "error": f"unknown command: {command}",
            }))
            sys.exit(1)

    except Exception as e:
        print(json.dumps({"status": "error", "error": f"stress plugin exception: {e}"}))
        sys.exit(1)

    # Return valid Protocol V2 response
    response = {
        "status": "ok",
        "result": json.dumps(result_data),
        "events": events,
        "state_updates": state_updates,
    }
    print(json.dumps(response))

if __name__ == "__main__":
    main()
