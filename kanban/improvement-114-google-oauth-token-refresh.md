# Improvement #114 — Google OAuth Token Refresh Plugin

**Type:** improvement
**ID:** 114
**Status:** backlog
**Created:** 2026-02-21
**Blocks:** #113 (YouTube playlist plugin)

---

## Goal

Build a scheduled ductile plugin that keeps a Google OAuth 2.0 access token fresh, writing it to a shared path on disk so other plugins (e.g. `youtube_playlist`) can use it without re-authenticating.

---

## Motivation

Google OAuth access tokens expire after 1 hour. Any plugin that calls Google APIs needs a valid token. Rather than each plugin handling refresh logic independently, a single scheduled plugin should own the token lifecycle and write a refreshed token to disk at a known path.

---

## Proposed Behavior

- **Schedule**: every 30–45 minutes (before token expiry)
- **On poll**: use the stored refresh token to obtain a new access token, write to `token_path`
- **On failure**: emit an alert event (so a pipeline can notify, log, etc.)

---

## Proposed Config

```yaml
plugins:
  google-oauth-refresh:
    enabled: true
    timeout: 30s
    schedule:
      every: 30m
      jitter: 5m
    config:
      client_id: "${GOOGLE_CLIENT_ID}"
      client_secret: "${GOOGLE_CLIENT_SECRET}"
      refresh_token: "${GOOGLE_REFRESH_TOKEN}"
      token_path: "/app/data/google_token.json"
      scopes:
        - "https://www.googleapis.com/auth/youtube"
```

---

## Setup Notes

- Initial OAuth consent must be done manually (browser flow) to obtain the refresh token
- Refresh token is long-lived (until revoked) — store in env var or secrets
- `GOOGLE_REFRESH_TOKEN` seeded into container via docker-compose environment or `.env` file
- Token file written to `/app/data/` (persisted volume) — readable by other plugins

---

## Notes

- Consider emitting a `google.token.refreshed` event on success so downstream plugins can react
- Token file should include expiry timestamp so consumers can validate before use
- May be generalised later to handle other Google API scopes (Drive, Calendar, etc.)
