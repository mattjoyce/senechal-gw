#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "requests>=2.31",
# ]
# ///

import datetime as dt
import json
import os
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

import requests


def respond(payload: Dict[str, Any]) -> None:
    json.dump(payload, sys.stdout)


def iso_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat()


def parse_iso(value: Optional[str]) -> Optional[dt.datetime]:
    if not value:
        return None
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def list_repos(owner: str, owner_type: str, token: Optional[str], include_private: bool) -> List[Dict[str, Any]]:
    session = requests.Session()
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "ductile-github-repo-sync",
    }
    if token:
        headers["Authorization"] = f"Bearer {token}"

    repos: List[Dict[str, Any]] = []
    endpoint = "orgs" if owner_type == "org" else "users"
    base_url = f"https://api.github.com/{endpoint}/{owner}/repos"
    repo_type = "all" if include_private else "public"

    page = 1
    while True:
        resp = session.get(
            base_url,
            headers=headers,
            params={
                "per_page": 100,
                "page": page,
                "type": repo_type,
                "sort": "updated",
                "direction": "desc",
            },
            timeout=20,
        )
        if resp.status_code != 200:
            raise RuntimeError(f"GitHub API error {resp.status_code}: {resp.text}")
        batch = resp.json()
        if not batch:
            break
        repos.extend(batch)
        page += 1
    return repos


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command") or "poll"
    config = request.get("config") or {}

    if command == "health":
        respond(
            {
                "status": "ok",
                "result": "github_repo_sync healthy",
                "logs": [{"level": "info", "message": "github_repo_sync healthy"}],
            }
        )
        return

    owner = config.get("owner") or os.getenv("GITHUB_OWNER")
    if not owner:
        respond(
            {
                "status": "error",
                "error": "missing required config: owner",
                "retry": False,
                "logs": [{"level": "error", "message": "missing required config: owner"}],
            }
        )
        return

    owner_type = config.get("owner_type", "user")
    clone_dir = Path(os.path.expanduser(config.get("clone_dir", "~/github.mattjoyce")))
    lookback_days = int(config.get("lookback_days", 730))
    include_private = bool(config.get("include_private", False))
    include_forks = bool(config.get("include_forks", True))
    token = config.get("github_token")
    token_env = config.get("github_token_env", "GITHUB_TOKEN")
    if not token:
        token = os.getenv(token_env) or os.getenv("GITHUB_TOKEN")

    clone_root = clone_dir / owner
    clone_root.mkdir(parents=True, exist_ok=True)

    now = dt.datetime.now(dt.timezone.utc)
    cutoff = now - dt.timedelta(days=lookback_days)

    logs: List[Dict[str, str]] = []
    skipped = 0

    try:
        repos = list_repos(owner, owner_type, token, include_private)
    except Exception as exc:
        respond(
            {
                "status": "error",
                "error": str(exc),
                "retry": True,
                "logs": [{"level": "error", "message": str(exc)}],
            }
        )
        return

    visible_repos = [
        repo
        for repo in repos
        if (include_private or not repo.get("private")) and (include_forks or not repo.get("fork"))
    ]

    events: List[Dict[str, Any]] = []
    queued = 0

    for repo in visible_repos:
        pushed_at = parse_iso(repo.get("pushed_at")) or parse_iso(repo.get("updated_at"))
        if pushed_at and pushed_at < cutoff:
            skipped += 1
            continue

        name = repo.get("name")
        clone_url = repo.get("clone_url")
        if not name or not clone_url:
            skipped += 1
            logs.append(
                {
                    "level": "warn",
                    "message": f"skipping repo with missing data: name={name}, clone_url={clone_url}",
                }
            )
            continue

        payload = {
            "owner": owner,
            "owner_type": owner_type,
            "repo_name": name,
            "full_name": repo.get("full_name"),
            "clone_url": clone_url,
            "ssh_url": repo.get("ssh_url"),
            "clone_dir": str(clone_dir),
            "default_branch": repo.get("default_branch"),
            "pushed_at": repo.get("pushed_at") or repo.get("updated_at"),
        }
        events.append({
            "type": "github_repo_sync.repo_discovered",
            "dedupe_key": f"git_repo_sync:{owner}/{name}",
            "payload": payload,
        })
        queued += 1

    total = len(visible_repos)
    summary = (
        f"GitHub repo discovery ({owner_type}:{owner}): queued {queued} repos for sync, "
        f"skipped {skipped} inactive, total {total}."
    )

    events.append(
        {
            "type": "github_repo_sync.completed",
            "payload": {
                "message": summary,
                "text": summary,
                "result": summary,
                "owner": owner,
                "owner_type": owner_type,
                "clone_dir": str(clone_dir),
                "queued": queued,
                "skipped": skipped,
                "total": total,
                "last_run": iso_now(),
            },
        }
    )

    respond(
        {
            "status": "ok",
            "result": summary,
            "events": events,
            "state_updates": {"last_run": iso_now(), "last_summary": summary},
            "logs": logs,
        }
    )


if __name__ == "__main__":
    main()
