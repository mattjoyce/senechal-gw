#!/usr/bin/env python3
import sys
import json
import time
import hashlib
import os
import urllib.request
import math

def get_input():
    if len(sys.argv) > 1:
        try:
            return json.loads(sys.argv[1])
        except json.JSONDecodeError:
            print(json.dumps({"error": "invalid JSON input"}), file=sys.stderr)
            sys.exit(1)
    return {}

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

def do_io(size_mb):
    path = "stress_test_data.tmp"
    chunk = b"X" * 1024 * 1024 # 1MB
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
    command = os.environ.get("DUCTILE_COMMAND", "health")
    input_data = get_input()
    payload = input_data.get("payload", {})
    state = input_data.get("state", {})

    result = {"status": "ok"}
    events = []

    if command == "cpu":
        duration = payload.get("duration_seconds", 5)
        actual = do_cpu(duration)
        result = {"result": "calculations complete", "actual_duration": actual}
        
    elif command == "io":
        size = payload.get("size_mb", 10)
        # Safety cap
        if size > 1000: size = 1000 
        digest, duration = do_io(size)
        result = {
            "hash": digest, 
            "write_speed_mbps": size / duration if duration > 0 else 0
        }

    elif command == "net":
        url = payload.get("url", "https://www.google.com")
        code, size = do_net(url)
        result = {"status_code": code, "bytes_received": size if isinstance(size, int) else 0}
        if isinstance(size, str):
            result["error"] = size

    elif command == "state":
        iterations = payload.get("iterations", 10)
        current_count = state.get("count", 0)
        
        for i in range(iterations):
            current_count += 1
            # Emit event every step to test event stream load
            emit_event("stress.state_increment", {"step": i, "total": current_count})
        
        result = {"final_count": current_count}
        # Update state for next run
        result["state"] = {"count": current_count}

    elif command == "health":
        result = {"status": "ok", "version": "0.1.0"}

    else:
        print(json.dumps({"error": f"unknown command: {command}"}), file=sys.stderr)
        sys.exit(1)

    print(json.dumps({"result": result}))

if __name__ == "__main__":
    main()
