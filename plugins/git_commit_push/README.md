# git_commit_push

Commit and push README/CHANGELOG updates with a Ductile author identity.

## Commands
- `handle` (write): Commit and push changes.
- `health` (read): Return health status.

## Input Payload
Accepts payload or context fields:
- `repo_path` or `path`
- `repo_name`
- `default_branch` (optional)
- `ssh_url` (optional)

## Configuration
Optional:
- `prefer_ssh`: Update origin to `ssh_url` before push.
- `ssh_alias_host`: SSH host alias to use instead of github.com (default: `github.com-ductile`).

## Behavior
- Skips if working tree is clean.
- Commits README/CHANGELOG changes with `[ductile-changelog]` tag.
- Pushes to `origin` on the default branch.

## Events
Emits `git_commit_push.completed` with payload:
- `repo_path`, `repo_name`, `changed`, `commit_sha`
