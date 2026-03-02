# git_repo_sync

Clone or fetch a single Git repository using event payload fields. Intended to run per-repo so each sync is queued.

## Commands
- `handle` (write): Clone or fetch a repository.
- `health` (read): Return health status.

## Input Payload
Expected fields from the triggering event:
- `owner`: GitHub owner.
- `repo_name`: Repository name.
- `clone_url`: Git clone URL.
- `ssh_url`: SSH clone URL (optional).
- `clone_dir`: Root directory for clones.

## Configuration
Optional:
- `prefer_ssh`: Use `ssh_url` when available and update origin to SSH.
- `ssh_alias_host`: SSH host alias to use instead of github.com (default: `github.com-ductile`).

## Events
Emits `git_repo_sync.completed` with payload fields:
- `message`: Human summary string for notifications/logging.
- `repo_name`, `owner`, `path`, `action`, `clone_url`, `ssh_url`

## Example Pipeline
```yaml
pipelines:
  - name: github-repo-sync
    on: github_repo_sync.repo_discovered
    steps:
      - id: sync
        uses: git_repo_sync
```
