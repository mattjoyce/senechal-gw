#!/usr/bin/env python3
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict


def respond(payload: Dict[str, Any]) -> None:
    json.dump(payload, sys.stdout)


def git_command(args, cwd: Path | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=str(cwd) if cwd else None, capture_output=True, text=True)


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command") or "handle"

    if command == "health":
        respond(
            {
                "status": "ok",
                "result": "git_repo_sync healthy",
                "logs": [{"level": "info", "message": "git_repo_sync healthy"}],
            }
        )
        return

    event = request.get("event") or {}
    payload = event.get("payload") or {}
    context = request.get("context") or {}

    def field(name: str):
        return payload.get(name) or context.get(name)

    owner = field("owner")
    repo_name = field("repo_name")
    clone_url = field("clone_url")
    clone_dir_raw = field("clone_dir") or "~/github.mattjoyce"

    if not owner or not repo_name or not clone_url:
        respond(
            {
                "status": "error",
                "error": "missing required payload fields (owner, repo_name, clone_url)",
                "retry": False,
                "logs": [
                    {
                        "level": "error",
                        "message": "missing required payload fields (owner, repo_name, clone_url)",
                    }
                ],
            }
        )
        return

    clone_dir = Path(os.path.expanduser(clone_dir_raw))
    repo_dir = clone_dir / owner / repo_name
    repo_dir.parent.mkdir(parents=True, exist_ok=True)

    action = "fetched"

    if repo_dir.exists() and (repo_dir / ".git").exists():
        result = git_command(["git", "-C", str(repo_dir), "fetch", "--prune", "--quiet"])
        if result.returncode != 0:
            respond(
                {
                    "status": "error",
                    "error": f"fetch failed: {result.stderr.strip()}",
                    "retry": True,
                    "logs": [
                        {
                            "level": "error",
                            "message": f"fetch failed for {repo_name}: {result.stderr.strip()}",
                        }
                    ],
                }
            )
            return
    elif repo_dir.exists():
        respond(
            {
                "status": "error",
                "error": f"path exists but is not a git repo: {repo_dir}",
                "retry": False,
                "logs": [
                    {
                        "level": "error",
                        "message": f"path exists but is not a git repo: {repo_dir}",
                    }
                ],
            }
        )
        return
    else:
        action = "cloned"
        result = git_command(["git", "clone", "--quiet", clone_url, str(repo_dir)])
        if result.returncode != 0:
            respond(
                {
                    "status": "error",
                    "error": f"clone failed: {result.stderr.strip()}",
                    "retry": True,
                    "logs": [
                        {
                            "level": "error",
                            "message": f"clone failed for {repo_name}: {result.stderr.strip()}",
                        }
                    ],
                }
            )
            return

    summary = f"Repo sync {action}: {owner}/{repo_name}"

    respond(
        {
            "status": "ok",
            "result": summary,
            "events": [
                {
                    "type": "git_repo_sync.completed",
                    "payload": {
                        "message": summary,
                        "text": summary,
                        "result": summary,
                        "owner": owner,
                        "repo_name": repo_name,
                        "path": str(repo_dir),
                        "action": action,
                    },
                }
            ],
            "logs": [{"level": "info", "message": summary}],
        }
    )


if __name__ == "__main__":
    main()
