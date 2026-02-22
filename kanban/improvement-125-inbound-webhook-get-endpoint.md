---
id: 125
status: todo
priority: Normal
blocked_by: []
tags: [improvement, webhook, oauth, api]
---

# Inbound Webhook GET Endpoint for Plugin Commands

## Motivation

Multiple existing standalone services follow the same pattern:

| Service | Callback URL | Token store | Refresh |
|---------|-------------|-------------|---------|
| Withings | `https://mattjoyce.ai/api/withings/` | `withings.db` | systemd/cron |
| Google OAuth | `localhost:9000/oauth/google/callback` | `google_oauth.db` | systemd timer |

Each is a FastAPI app whose sole job is: receive a browser redirect (GET + `?code=&state=`),
exchange the code for tokens, store them, return JSON. All are candidates for replacement
by a ductile plugin.

Currently ductile only exposes POST endpoints for plugin triggers. This card adds
a synchronous GET-capable inbound endpoint so plugins can own OAuth callbacks and
similar browser-redirect flows without a separate service — making this the general
solution for all OAuth integrations on this server.

## The Withings OAuth Callback Flow (Concrete Example)

### What Happens Today (withings container, now stopped)

```
User browser
  → visits Withings auth URL:
    https://account.withings.com/oauth2_user/authorize2
      ?client_id=...&redirect_uri=https://mattjoyce.ai/api/withings/&...
  → grants permission
  → Withings redirects browser to:
    https://mattjoyce.ai/api/withings/?code=ABC123&state=XYZ
  → Cloudflare tunnel routes that to http://192.168.20.4:9000
  → FastAPI app at port 9000:
      - extracts code and state from query params
      - POSTs to Withings API to exchange code for access+refresh tokens
      - writes tokens to withings.db
      - returns {"message": "OAuth successful!", "userid": ...}
```

The response is JSON — not a redirect or HTML — so no special browser rendering
is needed. The user just sees a success message.

### What Happens with This Feature

```
User browser
  → visits Withings auth URL (same as above)
  → Withings redirects browser to:
    https://mattjoyce.ai/api/withings/?code=ABC123&state=XYZ
  → Cloudflare tunnel routes that to:
    http://192.168.20.4:8888/webhook/withings/oauth_callback
  → Ductile inbound GET endpoint:
      - extracts query params → builds payload {code: "ABC123", state: "XYZ"}
      - executes withings plugin oauth_callback command SYNCHRONOUSLY
      - waits for result
      - returns plugin result as HTTP response body (JSON)
  → withings plugin oauth_callback command:
      - receives payload with code and state
      - POSTs to Withings API to exchange code for tokens
      - writes tokens to withings.db
      - returns {status: "ok", events: [{type: "withings.authorized", ...}]}
```

The Cloudflare tunnel config changes from:
- `https://mattjoyce.ai/api/withings/` → `http://192.168.20.4:9000`

To:
- `https://mattjoyce.ai/api/withings/` → `http://192.168.20.4:8888/webhook/withings/oauth_callback`

## Proposed Ductile API Change

Add a new synchronous GET endpoint:

```
GET /webhook/{plugin}/{command}?param1=value1&param2=value2
```

- Query parameters become the command payload (same shape as POST body `event.payload`)
- Executes the plugin command **synchronously** (blocks until complete)
- Returns the plugin result directly as the HTTP response
- Auth: configurable — could be unauthenticated (for public callbacks) or token-gated
- Only `handle`-type commands should be callable via this endpoint (read or write)

## Required Plugin Change: withings oauth_callback command

The withings plugin (`ductile-withings`) needs a new command:

```yaml
# manifest.yaml
- name: oauth_callback
  type: write
  description: "Handle Withings OAuth authorization callback. Exchanges code for tokens and stores in DB."
  input_schema:
    code: string    # required — authorization code from Withings
    state: string   # optional — CSRF state parameter
```

`run.py` implementation:
- Receive `code` from payload
- POST to `https://wbsapi.withings.net/v2/oauth2` with `grant_type=authorization_code`
- Write access_token, refresh_token, expires_at to `tokens` table
- Emit `withings.authorized` event
- Return success JSON

## Acceptance Criteria

- [ ] `GET /webhook/{plugin}/{command}` endpoint added to ductile API server
- [ ] Query params passed as payload to the plugin command
- [ ] Execution is synchronous — HTTP response waits for plugin result
- [ ] Auth policy configurable per webhook route (default: require token; allow unauthenticated for public callbacks)
- [ ] withings plugin has `oauth_callback` command implemented
- [ ] Cloudflare tunnel updated: `mattjoyce.ai/api/withings/` → ductile webhook endpoint
- [ ] Re-auth procedure documented in vault

## Notes

- This is needed rarely (initial setup or token emergency) but must work reliably when needed
- The withings container (`docker stop withings`) remains stopped; this feature lets us decommission it fully
- The `state` parameter in OAuth is a CSRF token — for now the plugin can log it but doesn't need to validate it (no session state in the plugin)
