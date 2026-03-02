# repo_policy

Ensure each repository has a README, a CHANGELOG, and a prominent README link to the changelog.

## Commands
- `handle` (write): Apply repo policy.
- `health` (read): Return health status.

## Input Payload
Accepts payload or context fields:
- `path` or `repo_path`: Path to the repo.
- `owner`, `repo_name`, `clone_dir`: Used to compute the path if `path` is missing.

## Events
Emits `repo_policy.completed` with payload:
- `repo_path`, `repo_name`, `changed`, `files_changed`

## Notes
Idempotent on repeated runs (no extra changes if already compliant).
