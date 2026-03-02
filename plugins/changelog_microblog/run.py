#!/usr/bin/env python3
import datetime as dt
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional


def respond(payload: Dict[str, Any]) -> None:
    json.dump(payload, sys.stdout)


def pick(payload: Dict[str, Any], context: Dict[str, Any], key: str) -> Optional[str]:
    if key in payload and payload[key] not in (None, ""):
        return payload[key]
    if key in context and context[key] not in (None, ""):
        return context[key]
    return None


def run_git(args: List[str], cwd: Path) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=str(cwd), capture_output=True, text=True)


def parse_iso(value: str) -> Optional[dt.datetime]:
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def is_vague(message: str) -> bool:
    lower = message.lower().strip()
    if not lower:
        return True
    if lower.startswith("merge"):
        return True
    words = [w for w in lower.replace("/", " ").split() if w]
    if len(words) < 3:
        return True
    vague_prefixes = {
        "fix",
        "fixes",
        "update",
        "updates",
        "chore",
        "cleanup",
        "refactor",
        "lint",
        "format",
        "docs",
        "readme",
        "wip",
    }
    if words and words[0] in vague_prefixes and len(words) <= 3:
        return True
    return False


def normalize_bullets(text: str) -> str:
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    bullets: List[str] = []
    for line in lines:
        if line.startswith("-") or line.startswith("*"):
            bullets.append(line)
        else:
            bullets.append(f"- {line}")
    return "\n".join(bullets)


def insert_changelog_entry(content: str, entry: str) -> str:
    if not content.strip():
        return f"# Changelog\n\n{entry}"

    lines = content.splitlines(keepends=True)
    if not lines:
        return f"# Changelog\n\n{entry}"

    if lines[0].startswith("#"):
        idx = 1
        while idx < len(lines) and lines[idx].strip() == "":
            idx += 1
        return "".join(lines[:idx]) + entry + "".join(lines[idx:])

    return f"# Changelog\n\n{entry}" + content


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command") or "handle"

    config = request.get("config") or {}
    event = request.get("event") or {}
    payload = event.get("payload") or {}
    context = request.get("context") or {}

    fabric_bin = config.get("fabric_bin", "fabric")
    fabric_pattern = config.get("fabric_pattern", "ductile-microblog-changelog")
    patterns_path = config.get("patterns_path", "~/.config/ductile/patterns")

    if command == "health":
        try:
            result = subprocess.run(
                [fabric_bin, "--help"], capture_output=True, text=True, timeout=10
            )
        except FileNotFoundError:
            respond(
                {
                    "status": "error",
                    "error": f"fabric binary not found: {fabric_bin}",
                    "retry": False,
                }
            )
            return
        if result.returncode != 0:
            respond(
                {
                    "status": "error",
                    "error": f"fabric --help failed: {result.stderr.strip()}",
                    "retry": False,
                }
            )
            return
        respond(
            {
                "status": "ok",
                "result": "changelog_microblog healthy",
                "logs": [{"level": "info", "message": "changelog_microblog healthy"}],
            }
        )
        return

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
    ssh_url = pick(payload, context, "ssh_url")
    clone_url = pick(payload, context, "clone_url")

    now = dt.datetime.now(dt.timezone.utc)
    default_start = now - dt.timedelta(days=7)

    last_changelog_time: Optional[dt.datetime] = None
    changelog_log = run_git(
        ["git", "log", "--format=%cI", "-n", "1", "--", "CHANGELOG.md"],
        cwd=repo_path,
    )
    if changelog_log.returncode == 0:
        raw = changelog_log.stdout.strip().splitlines()
        if raw:
            last_changelog_time = parse_iso(raw[0])

    if last_changelog_time and last_changelog_time > default_start:
        start_time = last_changelog_time
    else:
        start_time = default_start

    start_iso = start_time.isoformat()

    log_result = run_git(
        ["git", "log", "--since", start_iso, "--pretty=%s", "--reverse"],
        cwd=repo_path,
    )
    if log_result.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"git log failed: {log_result.stderr.strip()}",
                "retry": True,
                "logs": [{"level": "error", "message": "git log failed"}],
            }
        )
        return

    messages = []
    for line in log_result.stdout.splitlines():
        msg = line.strip()
        if not msg:
            continue
        if "[ductile-changelog]" in msg.lower():
            continue
        if is_vague(msg):
            continue
        messages.append(msg)

    if not messages:
        summary = "changelog_microblog: no substantive commits"
        respond(
            {
                "status": "ok",
                "result": summary,
                "events": [
                    {
                        "type": "changelog_microblog.completed",
                        "payload": {
                            "repo_path": str(repo_path),
                            "repo_name": repo_name,
                            "changed": False,
                            "entry_date": now.date().isoformat(),
                            "entry_text": "",
                            "since": start_iso,
                            "ssh_url": ssh_url or "",
                            "clone_url": clone_url or "",
                        },
                    }
                ],
                "logs": [{"level": "info", "message": summary}],
            }
        )
        return

    commit_block = "\n".join(f"- {msg}" for msg in messages)
    input_text = f"Repo: {repo_name}\nCommit messages:\n{commit_block}\n"

    env = os.environ.copy()
    env["FABRIC_PATTERNS_PATH"] = os.path.expanduser(patterns_path)

    pattern_arg = fabric_pattern
    pattern_path = Path(os.path.expanduser(fabric_pattern))
    if "/" in fabric_pattern or pattern_path.suffix == ".md":
        if pattern_path.exists():
            pattern_arg = str(pattern_path)
    else:
        candidate = Path(os.path.expanduser(patterns_path)) / f"{fabric_pattern}.md"
        if candidate.exists():
            pattern_arg = str(candidate)

    try:
        fabric_result = subprocess.run(
            [fabric_bin, "--pattern", pattern_arg],
            input=input_text,
            capture_output=True,
            text=True,
            timeout=120,
            env=env,
        )
    except FileNotFoundError:
        respond(
            {
                "status": "error",
                "error": f"fabric binary not found: {fabric_bin}",
                "retry": False,
                "logs": [{"level": "error", "message": "fabric binary not found"}],
            }
        )
        return
    except subprocess.TimeoutExpired:
        respond(
            {
                "status": "error",
                "error": "fabric execution timed out",
                "retry": True,
                "logs": [{"level": "error", "message": "fabric execution timed out"}],
            }
        )
        return

    if fabric_result.returncode != 0:
        respond(
            {
                "status": "error",
                "error": f"fabric failed: {fabric_result.stderr.strip()}",
                "retry": True,
                "logs": [{"level": "error", "message": "fabric failed"}],
            }
        )
        return

    entry_text = normalize_bullets(fabric_result.stdout.strip())
    if not entry_text:
        respond(
            {
                "status": "ok",
                "result": "changelog_microblog: no usable output",
                "events": [
                    {
                        "type": "changelog_microblog.completed",
                        "payload": {
                            "repo_path": str(repo_path),
                            "repo_name": repo_name,
                            "changed": False,
                            "entry_date": now.date().isoformat(),
                            "entry_text": "",
                            "since": start_iso,
                            "ssh_url": ssh_url or "",
                            "clone_url": clone_url or "",
                        },
                    }
                ],
                "logs": [{"level": "info", "message": "fabric produced empty output"}],
            }
        )
        return

    entry_date = now.date().isoformat()
    entry_block = f"## {entry_date}\n{entry_text}\n\n"

    changelog_path = repo_path / "CHANGELOG.md"
    if changelog_path.exists():
        content = changelog_path.read_text(encoding="utf-8")
    else:
        content = "# Changelog\n\n"

    updated = insert_changelog_entry(content, entry_block)
    if updated == content:
        summary = "changelog_microblog: no changelog changes"
        respond(
            {
                "status": "ok",
                "result": summary,
                "events": [
                    {
                        "type": "changelog_microblog.completed",
                        "payload": {
                            "repo_path": str(repo_path),
                            "repo_name": repo_name,
                            "changed": False,
                            "entry_date": entry_date,
                            "entry_text": entry_text,
                            "since": start_iso,
                            "ssh_url": ssh_url or "",
                            "clone_url": clone_url or "",
                        },
                    }
                ],
                "logs": [{"level": "info", "message": summary}],
            }
        )
        return

    changelog_path.write_text(updated, encoding="utf-8")
    summary = "changelog_microblog: updated CHANGELOG.md"

    respond(
        {
            "status": "ok",
            "result": summary,
            "events": [
                {
                    "type": "changelog_microblog.completed",
                    "payload": {
                        "repo_path": str(repo_path),
                        "repo_name": repo_name,
                        "changed": True,
                        "entry_date": entry_date,
                        "entry_text": entry_text,
                        "since": start_iso,
                        "ssh_url": ssh_url or "",
                        "clone_url": clone_url or "",
                    },
                }
            ],
            "logs": [{"level": "info", "message": summary}],
            "state_updates": {
                "last_run": now.isoformat(),
                "last_changelog_at": entry_date,
            },
        }
    )


if __name__ == "__main__":
    main()
