#!/usr/bin/env python3
import json
import os
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


def resolve_repo_path(payload: Dict[str, Any], context: Dict[str, Any]) -> Optional[Path]:
    raw_path = pick(payload, context, "path") or pick(payload, context, "repo_path")
    if raw_path:
        return Path(os.path.expanduser(raw_path))
    owner = pick(payload, context, "owner")
    repo_name = pick(payload, context, "repo_name")
    clone_dir = pick(payload, context, "clone_dir")
    if owner and repo_name and clone_dir:
        return Path(os.path.expanduser(clone_dir)) / owner / repo_name
    return None


def ensure_readme(repo_path: Path, repo_name: str, changelog_heading: str) -> List[Path]:
    changed: List[Path] = []
    readme_path = repo_path / "README.md"
    changelog_link = f"See [CHANGELOG.md](CHANGELOG.md)."
    section = f"## {changelog_heading}\n{changelog_link}\n"

    if not readme_path.exists():
        content = f"# {repo_name}\n\n{section}"
        readme_path.write_text(content, encoding="utf-8")
        changed.append(readme_path)
        return changed

    content = readme_path.read_text(encoding="utf-8")
    if "CHANGELOG.md" in content or "Changelog" in content or "CHANGELOG" in content:
        return changed

    updated = content.rstrip() + "\n\n" + section
    readme_path.write_text(updated, encoding="utf-8")
    changed.append(readme_path)
    return changed


def ensure_changelog(repo_path: Path, changelog_heading: str) -> List[Path]:
    changed: List[Path] = []
    changelog_path = repo_path / "CHANGELOG.md"
    if changelog_path.exists():
        return changed
    content = f"# {changelog_heading}\n\n"
    changelog_path.write_text(content, encoding="utf-8")
    changed.append(changelog_path)
    return changed


def main() -> None:
    request = json.load(sys.stdin)
    command = request.get("command") or "handle"

    if command == "health":
        respond(
            {
                "status": "ok",
                "result": "repo_policy healthy",
                "logs": [{"level": "info", "message": "repo_policy healthy"}],
            }
        )
        return

    config = request.get("config") or {}
    event = request.get("event") or {}
    payload = event.get("payload") or {}
    context = request.get("context") or {}

    repo_path = resolve_repo_path(payload, context)
    if not repo_path:
        respond(
            {
                "status": "error",
                "error": "missing repo path (path/repo_path or owner+repo_name+clone_dir)",
                "retry": False,
                "logs": [
                    {
                        "level": "error",
                        "message": "missing repo path (path/repo_path or owner+repo_name+clone_dir)",
                    }
                ],
            }
        )
        return

    if not repo_path.exists():
        respond(
            {
                "status": "error",
                "error": f"repo path does not exist: {repo_path}",
                "retry": False,
                "logs": [{"level": "error", "message": f"repo path does not exist: {repo_path}"}],
            }
        )
        return

    repo_name = pick(payload, context, "repo_name") or repo_path.name
    readme_heading = config.get("readme_heading", repo_name)
    changelog_heading = config.get("changelog_heading", "Changelog")

    changed_files: List[Path] = []
    changed_files += ensure_readme(repo_path, readme_heading, changelog_heading)
    changed_files += ensure_changelog(repo_path, changelog_heading)

    changed = len(changed_files) > 0
    files_changed = [str(path.relative_to(repo_path)) for path in changed_files]

    summary = "repo_policy: no changes"
    if changed:
        summary = f"repo_policy: updated {', '.join(files_changed)}"

    respond(
        {
            "status": "ok",
            "result": summary,
            "events": [
                {
                    "type": "repo_policy.completed",
                    "payload": {
                        "repo_path": str(repo_path),
                        "repo_name": repo_name,
                        "changed": changed,
                        "files_changed": files_changed,
                    },
                }
            ],
            "logs": [{"level": "info", "message": summary}],
        }
    )


if __name__ == "__main__":
    main()
