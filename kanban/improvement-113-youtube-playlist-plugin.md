# Improvement #113 — YouTube Playlist Plugin

**Type:** improvement
**ID:** 113
**Status:** backlog
**Created:** 2026-02-21
**Depends on:** #114 (Google OAuth token refresh)

---

## Goal

Build a ductile plugin that can read, add to, and remove items from a YouTube playlist via the YouTube Data API v3.

---

## Motivation

The existing `youtube_transcript` plugin fetches transcripts but has no YouTube account access. Managing playlists requires OAuth 2.0 authentication, which this plugin would leverage (via tokens managed by the `google-oauth-refresh` plugin, #114).

---

## Proposed Commands

| Command | Description |
|---------|-------------|
| `handle` | Main entry point — dispatched from pipeline events |
| `list` | Read all items in a playlist |
| `add` | Add a video to a playlist |
| `remove` | Remove a video from a playlist |
| `health` | Check API connectivity and token validity |

## Proposed Config

```yaml
plugins:
  youtube_playlist:
    enabled: true
    timeout: 30s
    config:
      playlist_id: "PLxxxxxxxxxxxx"
      token_path: "/app/data/google_token.json"  # managed by google-oauth-refresh
```

---

## API

- **YouTube Data API v3** — `playlistItems` resource
- Endpoints: `GET /playlistItems`, `POST /playlistItems`, `DELETE /playlistItems`
- Auth: OAuth 2.0 Bearer token (refreshed by plugin #114)
- Quota cost: ~1-3 units per operation (10,000 units/day free tier)

---

## Notes

- Requires OAuth consent for `youtube` scope
- Token refresh must be handled by a separate scheduled plugin (#114) to avoid re-auth on every run
- Consider supporting multiple playlists via event baggage (pass `playlist_id` at trigger time)
