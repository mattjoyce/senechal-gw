# /// script
# dependencies = [
#   "requests>=2.31",
# ]
# ///

import sys
import json
import os
import datetime
import requests

def main():
    try:
        request_data = sys.stdin.read()
        if not request_data:
            return
        req = json.loads(request_data)
    except Exception as e:
        print(json.dumps({"status": "error", "error": f"Failed to parse request: {str(e)}"}), flush=True)
        return

    command = req.get("command", "poll")
    config = req.get("config", {})
    api_key = config.get("anthropic_api_key")

    if not api_key:
        print(json.dumps({
            "status": "error", 
            "error": "anthropic_api_key is required in config",
            "logs": [{"level": "error", "message": "Missing anthropic_api_key"}]
        }), flush=True)
        return

    # Note: For full cost/usage reports, Anthropic requires an Admin API Key (sk-ant-admin...)
    # Standard keys might fail on /v1/admin/ endpoints.
    
    headers = {
        "x-api-key": api_key,
        "anthropic-version": "2023-06-01",
        "content-type": "application/json"
    }

    if command in ["poll", "summary"]:
        # Get usage for the current month
        today = datetime.date.today()
        first_of_month = today.replace(day=1)
        
        # We'll try to get cost data. 
        # API Reference: https://docs.anthropic.com/en/api/admin-api
        try:
            # Note: The exact endpoint for the new Admin API is often /v1/admin/costs
            # or /v1/organizations/usage_report/messages
            # We'll use the messages usage report as a fallback or primary.
            
            url = "https://api.anthropic.com/v1/admin/costs"
            params = {
                "start_date": first_of_month.isoformat(),
                "end_date": today.isoformat()
            }
            
            resp = requests.get(url, headers=headers, params=params)
            
            if resp.status_code == 200:
                data = resp.json()
                # Process data... (simplified for MVP)
                total_cost = 0.0
                if "costs" in data:
                    for item in data["costs"]:
                        total_cost += float(item.get("amount", 0))
                
                print(json.dumps({
                    "status": "ok",
                    "state_updates": {
                        "total_cost_month_usd": total_cost,
                        "last_check": datetime.datetime.now().isoformat()
                    },
                    "logs": [{"level": "info", "message": f"Fetched usage: ${total_cost:.2f} spent this month"}]
                }), flush=True)
            elif resp.status_code == 404:
                # Fallback or Admin API not enabled/wrong key type
                 print(json.dumps({
                    "status": "error",
                    "error": f"Endpoint not found (404). Note: Admin API requires sk-ant-admin key. Code: {resp.status_code}",
                    "logs": [{"level": "warn", "message": f"Admin API endpoint returned 404. Response: {resp.text}"}]
                }), flush=True)
            else:
                print(json.dumps({
                    "status": "error",
                    "error": f"Anthropic API error: {resp.status_code} - {resp.text}",
                    "logs": [{"level": "error", "message": f"API Failure: {resp.text}"}]
                }), flush=True)
                
        except Exception as e:
            print(json.dumps({
                "status": "error",
                "error": f"Plugin execution failed: {str(e)}",
                "logs": [{"level": "error", "message": str(e)}]
            }), flush=True)
    else:
        print(json.dumps({
            "status": "error",
            "error": f"Unknown command: {command}"
        }), flush=True)

if __name__ == "__main__":
    main()
