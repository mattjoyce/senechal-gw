# discord_notify

Send a message to a Discord channel via an incoming webhook. Reads content from `payload.message`, `payload.content`, `payload.result`, or `payload.title` (in that order) with context fallback.

## Commands
- `handle` (write): Post a message to Discord.
- `poll` (write): Scheduled alias for `handle`.
- `health` (read): Validate webhook configuration.

## Configuration
- `webhook_url` (required): Discord webhook URL.
- `default_username`: Username override (default: `Ductile`).
- `default_avatar_url`: Avatar URL override.
- `poll_message`: Default message for scheduled polls.
- `request_timeout_seconds`: HTTP timeout (default: 10).

## Example
```yaml
plugins:
  discord_notify:
    enabled: true
    config:
      webhook_url: ${DISCORD_WEBHOOK_URL}
      default_username: "Ductile"
      poll_message: "Daily heartbeat"
```

Example payload:
```json
{ "message": "Deployment completed" }
```
