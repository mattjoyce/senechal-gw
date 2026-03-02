# github_repo_sync

Discover public GitHub repositories updated within a lookback window and emit per-repo events so each sync runs as a queued job. Also emits a summary event for notifications.

## Commands
- `poll` (write): Discover repos and emit sync events.
- `handle` (write): Alias for `poll`.
- `health` (read): Return health status.

## Configuration
Required:
- `owner`: GitHub user or org name.

Optional:
- `owner_type`: `user` or `org` (default: `user`).
- `clone_dir`: Local root for clones (default: `~/github.mattjoyce`).
- `lookback_days`: Include repos updated within this many days (default: 730).
- `include_private`: Include private repos (default: false).
- `github_token`: GitHub token value.
- `github_token_env`: Environment variable name to read the token from (default: `GITHUB_TOKEN`).
- `include_forks`: Include forked repos (default: true).

## Events
Emits events:
- `github_repo_sync.repo_discovered` for each eligible repo, with payload:
  - `owner`, `owner_type`, `repo_name`, `full_name`, `clone_url`, `ssh_url`, `clone_dir`, `default_branch`, `pushed_at`
- `github_repo_sync.completed` with payload fields:
  - `message`: Human summary string for notifications.
  - `owner`, `owner_type`, `clone_dir`
  - `queued`, `skipped`, `total`

## Example
```yaml
plugins:
  github_repo_sync:
    enabled: true
    schedules:
      - id: daily-early
        cron: "0 6 * * *"
        timezone: "Australia/Sydney"
    config:
      owner: "mattjoyce"
      owner_type: "user"
      clone_dir: "~/github.mattjoyce"
      lookback_days: 730
      include_private: false
      github_token_env: "GITHUB_TOKEN"
```
