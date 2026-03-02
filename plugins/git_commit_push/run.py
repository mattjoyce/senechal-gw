#!/usr/bin/env python3
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, Optional
from urllib.parse import urlparse


def respond(payload: Dict[str, Any]) -> None:
    json.dump(payload, sys.stdout)


def pick(payload: Dict[str, Any], context: Dict[str, Any], key: str) -> Optional[str]:
    if key in payload and payload[key] not in (None, ""):
        return payload[key]
    if key in context and context[key] not in (None, ""):
        return context[key]
    return None


def run_git(args, cwd: Path, env: Optional[Dict[str, str]] = None) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=str(cwd), capture_output=True, text=True, env=env)


def rewrite_ssh_url(url: str | None, alias_host: str) -> str | None:
    if not url:
        return None
    if not alias_host or alias_host == "github.com":
        return url
    if url.startswith("git@"):
        if ":" in url:
            user_host, path = url.split(":", 1)
            user, _host = user_host.split("@", 1)
            return f"{user}@{alias_host}:{path}"
    if url.startswith("ssh://"):
        parsed = urlparse(url)
        if parsed.hostname:
            user = parsed.username or "git"
            path = parsed.path.lstrip("/")
            return f"ssh://{user}@{alias_host}/{path}"
    return url


def https_to_ssh_alias(url: str | None, alias_host: str) -> str | None:
    if not url or not alias_host:
        return None
    parsed = urlparse(url)
    if parsed.scheme not in {"http", "https"}:
        return None
    if parsed.hostname not in {"github.com", "www.github.com"}:
        return None
    path = parsed.path.lstrip("/")
    if not path:
        return None
    if not path.endswith(".git"):
        path = f"{path}.git"
    return f"git@{alias_host}:{path}"


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command") or "handle"

    if command == "health":
        respond(
            {
                "status": "ok",
                "result": "git_commit_push healthy",
                "logs": [{"level": "info", "message": "git_commit_push healthy"}],
            }
        )
        return

    config = request.get("config") or {}
    event = request.get("event") or {}
    payload = event.get("payload") or {}
    context = request.get("context") or {}

    repo_path_raw = pick(payload, context, "repo_path") or pick(payload, context, "path")
    if not repo_path_raw:
        respond(
            {
                "status": "error",
                "error": "missing repo_path/path",
                "retry": False,
                "logs": [{"level": "error", "message": "missing repo_path/path"}],
            }
        )
        return

    repo_path = Path(os.path.expanduser(repo_path_raw))
    if not repo_path.exists() or not (repo_path / ".git").exists():
        respond(
            {
                "status": "error",
                "error": f"repo path invalid: {repo_path}",
                "retry": False,
                "logs": [{"level": "error", "message": f"repo path invalid: {repo_path}"}],
            }
        )
        return

    repo_name = pick(payload, context, "repo_name") or repo_path.name
    default_branch = pick(payload, context, "default_branch") or "main"
    ssh_url = pick(payload, context, "ssh_url")
    prefer_ssh = bool(config.get("prefer_ssh", False))
    ssh_alias_host = config.get("ssh_alias_host", "github.com-ductile")
    ssh_url_effective = rewrite_ssh_url(ssh_url, ssh_alias_host) if prefer_ssh else ssh_url

    status = run_git(["git", "status", "--porcelain"], cwd=repo_path)
    if status.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"git status failed: {status.stderr.strip()}",
                "retry": True,
                "logs": [{"level": "error", "message": "git status failed"}],
            }
        )
        return

    status_lines = [line for line in status.stdout.splitlines() if line.strip()]
    if status_lines:
        allowed = {"README.md", "CHANGELOG.md"}
        changed_files = {line[3:].strip() for line in status_lines if len(line) > 3}
        if any(path not in allowed for path in changed_files):
            summary = "git_commit_push: skipped dirty working tree"
            respond(
                {
                    "status": "ok",
                    "result": summary,
                    "events": [],
                    "logs": [
                        {
                            "level": "warn",
                            "message": f"{summary}: {', '.join(sorted(changed_files))}",
                        }
                    ],
                }
            )
            return

    if not status.stdout.strip():
        summary = "git_commit_push: no changes to commit"
        respond(
            {
                "status": "ok",
                "result": summary,
                "events": [],
                "logs": [{"level": "info", "message": summary}],
            }
        )
        return

    branch_result = run_git(["git", "rev-parse", "--abbrev-ref", "HEAD"], cwd=repo_path)
    current_branch = branch_result.stdout.strip() if branch_result.returncode == 0 else ""

    if current_branch and current_branch != default_branch:
        warn = f"git_commit_push: repo on {current_branch}, expected {default_branch}; committing on current branch"
    else:
        warn = ""

    env = os.environ.copy()
    author_name = config.get("author_name", "ductile")
    author_email = config.get("author_email", "ductile@local")
    env.update(
        {
            "GIT_AUTHOR_NAME": author_name,
            "GIT_AUTHOR_EMAIL": author_email,
            "GIT_COMMITTER_NAME": author_name,
            "GIT_COMMITTER_EMAIL": author_email,
        }
    )

    add_result = run_git(["git", "add", "README.md", "CHANGELOG.md"], cwd=repo_path, env=env)
    if add_result.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"git add failed: {add_result.stderr.strip()}",
                "retry": False,
                "logs": [{"level": "error", "message": "git add failed"}],
            }
        )
        return

    staged = run_git(["git", "diff", "--cached", "--name-only"], cwd=repo_path)
    if staged.returncode != 0 or not staged.stdout.strip():
        summary = "git_commit_push: no staged changes"
        respond(
            {
                "status": "ok",
                "result": summary,
                "events": [],
                "logs": [{"level": "info", "message": summary}],
            }
        )
        return

    commit_message = config.get("commit_message", "Update changelog [ductile-changelog]")
    commit_result = run_git(["git", "commit", "-m", commit_message], cwd=repo_path, env=env)
    if commit_result.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"git commit failed: {commit_result.stderr.strip()}",
                "retry": False,
                "logs": [{"level": "error", "message": "git commit failed"}],
            }
        )
        return

    sha_result = run_git(["git", "rev-parse", "HEAD"], cwd=repo_path)
    commit_sha = sha_result.stdout.strip() if sha_result.returncode == 0 else ""

    if prefer_ssh and not ssh_url_effective:
        origin_url = run_git(["git", "remote", "get-url", "origin"], cwd=repo_path)
        if origin_url.returncode == 0:
            candidate = origin_url.stdout.strip()
            ssh_url_effective = rewrite_ssh_url(candidate, ssh_alias_host)
            if not ssh_url_effective or ssh_url_effective == candidate:
                ssh_url_effective = https_to_ssh_alias(candidate, ssh_alias_host)

    if prefer_ssh and ssh_url_effective:
        remote_result = run_git(
            ["git", "remote", "set-url", "origin", ssh_url_effective], cwd=repo_path
        )
        if remote_result.returncode != 0:
            respond(
                {
                    "status": "error",
                    "error": f"git remote set-url failed: {remote_result.stderr.strip()}",
                    "retry": True,
                    "logs": [
                        {
                            "level": "error",
                            "message": "git remote set-url failed",
                        }
                    ],
                }
            )
            return
    elif prefer_ssh and not ssh_url_effective:
        warn = warn or "git_commit_push: prefer_ssh enabled but ssh_url missing"

    push_target = current_branch or default_branch
    push_result = run_git(["git", "push", "origin", push_target], cwd=repo_path)
    if push_result.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"git push failed: {push_result.stderr.strip()}",
                "retry": True,
                "logs": [{"level": "error", "message": "git push failed"}],
            }
        )
        return

    summary = f"git_commit_push: pushed {repo_name}"
    logs = [{"level": "info", "message": summary}]
    if warn:
        logs.append({"level": "warn", "message": warn})

    respond(
        {
            "status": "ok",
            "result": summary,
            "events": [
                {
                    "type": "git_commit_push.completed",
                    "payload": {
                        "repo_path": str(repo_path),
                        "repo_name": repo_name,
                        "changed": True,
                        "commit_sha": commit_sha,
                    },
                }
            ],
            "logs": logs,
        }
    )


if __name__ == "__main__":
    main()
