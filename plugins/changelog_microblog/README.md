# changelog_microblog

Generate a micro-blog style changelog entry from recent commit messages and append it to CHANGELOG.md.

## Commands
- `handle` (write): Generate and append changelog entries.
- `health` (read): Return health status.

## Input Payload
Accepts payload or context fields:
- `repo_path` or `path`: repo path
- `repo_name`
- `default_branch`

## Behavior
- Lookback starts at the most recent of (7 days ago) or the last changelog update.
- Skips vague commits and commits tagged `[ductile-changelog]`.
- Uses a fabric pattern to turn commits into micro-blog bullets (absolute pattern path supported).

## Events
Emits `changelog_microblog.completed` with payload:
- `repo_path`, `repo_name`, `changed`, `entry_date`, `entry_text`
