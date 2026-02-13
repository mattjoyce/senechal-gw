---
id: 35
status: done
priority: High
blocked_by: []
tags: [sprint-3, security, api, auth, webhooks]
---

# API Token Scopes (Fine-Grained Authorization)

Implement scope-based authorization for API tokens. Allows limiting tokens to specific plugins, commands, or read-only access. Prevents accidentally triggering wrong plugins or overly permissive external integrations.

## Motivation

**Current state (Sprint 2):** Single `api_key` grants full access to all endpoints.

**Problems:**
1. **Webhook 3rd parties (Sprint 3)** - External services need API access for status checks, but shouldn't trigger unrelated plugins
   - GitHub webhooks might need to query job results, but shouldn't trigger Withings polls
   - OAuth callback handlers need to update plugin state, but shouldn't have admin access
2. **External integrations** - If you give a token to a monitoring service, it can trigger any plugin
3. **Accidents** - No guard rails against `curl -X POST /trigger/delete-all-data/run`
4. **Least privilege** - Can't give read-only access for monitoring/TUI
5. **Multi-user** - Can't safely share gateway with collaborators

**Solution:** Token scopes like `read:*`, `write:github-handler:*`, `write:withings:poll`

**Urgency:** Sprint 3 (webhooks) will require distributing tokens to external services. Without scopes, every webhook integration gets god-mode access.

## HMAC vs Bearer Token Authentication

**Two different auth mechanisms for different purposes:**

**HMAC-SHA256 (Webhook POSTs):**
- Authenticates inbound webhook payloads from 3rd parties
- Configured per webhook endpoint in `config.yaml`
- Verifies the sender is who they claim to be
- Example: GitHub signs webhook body with shared secret
- **Protects:** POST /webhook/{path}

**Bearer Token + Scopes (API Access):**
- Authenticates API requests (job triggers, status queries, events)
- Configured in `api.auth.tokens` array
- Limits what actions the token holder can perform
- Example: GitHub queries job status after webhook delivery
- **Protects:** POST /trigger, GET /job, GET /events, etc.

**Why both?**
A 3rd party service (like GitHub) might:
1. Send webhooks to Ductile (authenticated via HMAC)
2. Query job results via API (authenticated via Bearer token with scopes)
3. Trigger follow-up actions (authenticated via Bearer token with scopes)

Without scopes on (2) and (3), you'd have to give GitHub full API access just to check status.

## Acceptance Criteria

- Token config includes optional `scopes` array
- Scopes checked before allowing API request
- Scope syntax: `<action>:<plugin>:<command>` where `*` is wildcard
- Actions: `read` (GET endpoints), `write` (POST endpoints), `admin` (reload, reset)
- Authorization failure returns 403 with minimal detail ("insufficient scope")
- Default: no scopes = full access (backward compatible)
- Scope validation at middleware layer, before handler logic

## Scope Syntax

**Two layers:** High-level manifest-driven shorthands + low-level granular control.

### Layer 1: Manifest-Driven Shorthands (Recommended)

**Format:** `<plugin>:ro|rw` or `<plugin>:allow:<command>` or `<plugin>:deny:<command>`

Scopes reference the plugin's manifest to determine which commands are safe (read-only) vs have side effects (read-write).

**Examples:**
```yaml
# All read-only commands for withings (as defined in manifest)
scopes: ["withings:ro"]

# All commands for withings (read-only + read-write)
scopes: ["withings:rw"]

# Specific command (regardless of ro/rw classification)
scopes: ["withings:allow:poll"]

# Explicit deny (overrides other grants)
scopes: ["withings:rw", "withings:deny:sync"]
```

**How it works:**
1. Plugin manifest declares which commands are `read` vs `write`:
   ```yaml
   # plugins/withings/manifest.yaml
   commands:
     poll:
       type: read
       description: "Fetch latest measurements"
     sync:
       type: write
       description: "Push data to Withings API"
     oauth_callback:
       type: write
       description: "Handle OAuth callback"
   ```

2. Scope `withings:ro` expands to: `["trigger:withings:poll"]`
3. Scope `withings:rw` expands to: `["trigger:withings:poll", "trigger:withings:sync", "trigger:withings:oauth_callback"]`

**Benefits:**
- **DRY:** Don't duplicate command lists in manifest and token config
- **Self-documenting:** Plugins declare their own security model
- **QoL:** Easy to grant "all safe operations" without listing each one
- **Evolvable:** Add new commands to plugin, ro/rw scopes automatically include them

### Layer 2: Low-Level Granular Control

**Format:** `<action>:<resource>:<command>`

Direct control over API endpoints, not tied to manifests. Used for system resources (jobs, events, healthz) and fine-grained overrides.

**Actions:**
- `read` - GET /job/{id}, GET /healthz, GET /events, GET /plugins, GET /queue
- `trigger` - POST /trigger/{plugin}/{command}
- `admin` - POST /reload, POST /reset/{plugin}

**Resources:**
- `*` - All resources
- `{plugin}` - Specific plugin name
- `jobs`, `events`, `healthz`, `queue` - System resource types

**Commands:**
- `*` - All commands (for plugin resources)
- `{command}` - Specific command name

**Examples:**
```yaml
# Read-only system access (no plugin triggers)
scopes: ["read:*"]

# Can only trigger withings polls (low-level syntax)
scopes: ["trigger:withings:poll"]

# Read job status + trigger any withings command
scopes: ["read:jobs", "trigger:withings:*"]

# Full access (equivalent to no scopes)
scopes: ["read:*", "trigger:*:*", "admin:*"]
```

### Combining Both Layers

```yaml
tokens:
  - key: ${GITHUB_TOKEN}
    name: "github-integration"
    scopes:
      - "read:jobs"          # Can check job status (low-level)
      - "read:events"        # Can subscribe to events (low-level)
      - "withings:ro"        # Can trigger safe withings commands (manifest-driven)
      - "garmin:allow:sync"  # Can trigger specific garmin command (manifest-driven)
      - "slack:deny:*"       # Explicit deny, even if granted elsewhere

  - key: ${MONITORING_TOKEN}
    name: "grafana"
    scopes:
      - "read:*"             # All GET endpoints, no triggers

  - key: ${EXTERNAL_CRON}
    name: "external-cron"
    scopes:
      - "withings:ro"        # Only safe, read-only operations
```

### Scope Precedence

1. **Explicit deny** - `plugin:deny:command` overrides all grants
2. **Explicit allow** - `plugin:allow:command` or `trigger:plugin:command`
3. **Manifest-driven ro/rw** - Expands based on manifest
4. **Default deny** - No scope = no access (unless legacy single api_key)

## Configuration

```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    tokens:
      # Full access token (for admin/CLI)
      - key: ${ADMIN_API_KEY}
        name: "admin-cli"
        scopes: ["read:*", "trigger:*:*", "admin:*"]

      # Monitoring-only token (for TUI/Grafana)
      - key: ${MONITOR_API_KEY}
        name: "tui-monitor"
        scopes: ["read:*"]

      # Webhook 3rd party (GitHub integration)
      - key: ${GITHUB_API_KEY}
        name: "github-integration"
        scopes:
          - "read:jobs"           # Check job status
          - "read:events"         # Subscribe to event stream
          - "github-handler:rw"   # Trigger github-handler commands

      # External cron (Withings polling only)
      - key: ${WITHINGS_CRON_KEY}
        name: "external-cron"
        scopes:
          - "withings:ro"         # Only read-only withings commands (poll)

      # OAuth callback handler
      - key: ${OAUTH_CALLBACK_KEY}
        name: "oauth-callback"
        scopes:
          - "withings:allow:oauth_callback"  # Specific command only
          - "garmin:allow:oauth_callback"

      # Read-only events (for debugging)
      - key: ${EVENTS_KEY}
        name: "event-stream"
        scopes: ["read:events"]
```

**Backward Compatibility:**
```yaml
# Old config style (still works, grants full access)
api:
  auth:
    api_key: ${API_KEY}
```

## Implementation Details

**Package:** `internal/api` (extend existing auth)

**Token Registry:**
```go
type Token struct {
    Key    string
    Name   string
    Scopes []Scope
}

type Scope struct {
    Action   string  // read, write, admin
    Resource string  // plugin name, *, jobs, events, etc.
    Command  string  // command name, *
}

func ParseScope(s string) (Scope, error) {
    parts := strings.Split(s, ":")
    if len(parts) != 3 {
        return Scope{}, fmt.Errorf("invalid scope format: %s", s)
    }
    return Scope{Action: parts[0], Resource: parts[1], Command: parts[2]}, nil
}

func (s Scope) Matches(action, resource, command string) bool {
    return (s.Action == "*" || s.Action == action) &&
           (s.Resource == "*" || s.Resource == resource) &&
           (s.Command == "*" || s.Command == command)
}
```

**Auth Middleware Enhancement:**
```go
type AuthContext struct {
    Token  *Token
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract bearer token
        authHeader := r.Header.Get("Authorization")
        token := extractToken(authHeader)

        // Find token in registry
        t := s.tokenRegistry.Get(token)
        if t == nil {
            http.Error(w, "unauthorized", 401)
            return
        }

        // Add to request context
        ctx := context.WithValue(r.Context(), authContextKey, &AuthContext{Token: t})
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func (s *Server) requireScope(action, resource, command string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            auth := r.Context().Value(authContextKey).(*AuthContext)

            // Check if any scope matches
            if !auth.Token.HasScope(action, resource, command) {
                http.Error(w, "forbidden", 403)
                s.logger.Warn("insufficient scope",
                    "token", auth.Token.Name,
                    "required", fmt.Sprintf("%s:%s:%s", action, resource, command))
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

**Route Middleware Application:**
```go
// GET endpoints - require read scope
r.With(s.requireScope("read", "jobs", "*")).Get("/job/{id}", s.handleGetJob)
r.With(s.requireScope("read", "events", "*")).Get("/events", s.handleEvents)
r.With(s.requireScope("read", "healthz", "*")).Get("/healthz", s.handleHealthz)

// POST trigger - require write scope for specific plugin/command
r.Post("/trigger/{plugin}/{command}", func(w http.ResponseWriter, r *http.Request) {
    plugin := chi.URLParam(r, "plugin")
    command := chi.URLParam(r, "command")

    // Check scope dynamically
    auth := r.Context().Value(authContextKey).(*AuthContext)
    if !auth.Token.HasScope("write", plugin, command) {
        http.Error(w, "forbidden", 403)
        return
    }

    s.handleTrigger(w, r)
})

// Admin endpoints
r.With(s.requireScope("admin", "*", "*")).Post("/reload", s.handleReload)
r.With(s.requireScope("admin", "*", "*")).Post("/reset/{plugin}", s.handleReset)
```

**Token Helper Methods:**
```go
func (t *Token) HasScope(action, resource, command string) bool {
    // Empty scopes = full access (backward compat)
    if len(t.Scopes) == 0 {
        return true
    }

    // Check if any scope matches
    for _, scope := range t.Scopes {
        if scope.Matches(action, resource, command) {
            return true
        }
    }
    return false
}
```

## Security Considerations

**1. Minimal Error Messages**
- 403 response body: `{"error": "forbidden"}` (no scope details)
- Log the attempted action server-side for audit
- Prevents token scope enumeration

**2. Scope Validation at Parse Time**
- Reject invalid scope syntax at config load
- Fail fast rather than silently granting full access

**3. No Scope Escalation**
- Tokens cannot grant themselves new scopes
- No `/token/create` endpoint in v1

**4. Audit Logging**
- Log token name + action on every request (success and failure)
- Enables tracking which integration is triggering which plugins

**Example log:**
```json
{
  "level": "info",
  "msg": "api_request",
  "token": "external-cron",
  "action": "write:withings:poll",
  "remote_addr": "192.168.1.100",
  "status": 202
}
```

## Testing

**Unit Tests:**
```go
func TestScopeMatching(t *testing.T) {
    tests := []struct {
        scope    string
        action   string
        resource string
        command  string
        matches  bool
    }{
        {"read:*", "read", "jobs", "poll", true},
        {"read:*", "write", "jobs", "poll", false},
        {"write:withings:poll", "write", "withings", "poll", true},
        {"write:withings:poll", "write", "withings", "handle", false},
        {"write:*:poll", "write", "garmin", "poll", true},
    }
    // ...
}
```

**Integration Tests:**
```go
func TestScopeEnforcement(t *testing.T) {
    // Token with read-only scope
    token := createToken(t, "read-only", []string{"read:*"})

    // GET /healthz - should succeed
    resp := get(t, "/healthz", token)
    assert.Equal(t, 200, resp.StatusCode)

    // POST /trigger/echo/poll - should fail with 403
    resp = post(t, "/trigger/echo/poll", token, nil)
    assert.Equal(t, 403, resp.StatusCode)
}
```

## Migration Path

**Phase 1 (Backward Compatible):**
- Add scope support, but `api_key` string still works (treated as full access)
- New `tokens` array is optional

**Phase 2 (Deprecation Warning):**
- Log warning if using old `api_key` format
- Documentation recommends migrating to `tokens` array

**Phase 3 (Future):**
- Drop support for `api_key` string (breaking change, major version bump)

## Use Cases

**1. Webhook 3rd Party Integration (PRIMARY):**
```yaml
# GitHub webhook needs to query job status after webhook delivery
# HMAC protects the POST /webhook/github endpoint
# Bearer token protects the GET /job/{id} query
webhooks:
  - path: /webhook/github
    secret: ${GITHUB_WEBHOOK_SECRET}  # HMAC signature
    plugin: github-handler
    command: handle

api:
  auth:
    tokens:
      - key: ${GITHUB_API_TOKEN}  # For status queries
        name: "github-integration"
        scopes: ["read:jobs", "read:healthz"]  # Can check job results, nothing else
```

**Why both HMAC and scopes?**
- **HMAC** authenticates the inbound webhook POST (GitHub → Ductile)
- **Bearer token** authenticates status queries (GitHub → Ductile API)
- Without scopes, GitHub could trigger unrelated plugins via the API

**2. OAuth Callback Handler:**
```yaml
# Plugin handles OAuth callback, needs to update its own state
# But shouldn't have admin access or trigger other plugins
tokens:
  - key: ${OAUTH_CALLBACK_TOKEN}
    name: "oauth-callback"
    scopes: ["write:withings:oauth_callback", "read:healthz"]
```

**3. External Cron Job:**
```yaml
# Give external system minimal scope
tokens:
  - key: ${CRON_TOKEN}
    name: "external-cron"
    scopes: ["write:withings:poll"]
```

**4. Monitoring Dashboard:**
```yaml
# Grafana can read metrics but not trigger jobs
tokens:
  - key: ${GRAFANA_TOKEN}
    name: "grafana"
    scopes: ["read:healthz", "read:jobs"]
```

**5. TUI Monitor:**
```yaml
# Full read access for local monitoring
tokens:
  - key: ${TUI_TOKEN}
    name: "tui-monitor"
    scopes: ["read:*"]
```

**6. Multi-User Access:**
```yaml
# Collaborator can only trigger their own plugins
tokens:
  - key: ${USER_A_TOKEN}
    name: "user-a"
    scopes: ["write:plugin-a:*", "read:*"]

  - key: ${USER_B_TOKEN}
    name: "user-b"
    scopes: ["write:plugin-b:*", "read:*"]
```

## Dependencies

- Existing API server (Sprint 2 ✓)
- Existing auth middleware (Sprint 2 ✓)
- **Manifest command type metadata (#36)** - Required for ro/rw scope shorthands. Low-level scopes (action:plugin:command) work without this.
- **Sprint 3 webhooks** - Token scopes should be implemented alongside or before webhooks to avoid distributing overly-permissive tokens

**Recommendation:** Implement #36 first (manifest metadata), then #35 (scopes), then webhook integrations. This ensures manifest-driven scopes are available when configuring 3rd party integrations.

## Deferred Features

**Not in v1:**
- Token expiration / rotation (use external secret manager)
- Rate limiting per token (proxy responsibility)
- Dynamic scope grant/revoke API (just edit config.yaml)
- Scope inheritance / hierarchies (YAGNI)

## Narrative

- 2026-02-10: Implemented as part of PR #12. Manifest-driven scopes, authorization middleware, 403 on insufficient scope. All tests passing. (by @codex)

Scopes solve two critical problems for Sprint 3:

**1. Webhook 3rd Party Security:** When you configure GitHub to send webhooks to Ductile, GitHub might also need API access to query job status or trigger related actions. Without scopes, you'd have to choose between:
- Giving GitHub a god-mode token (security nightmare)
- Not giving GitHub any API access (breaks status callback pattern)

With scopes, GitHub gets `["read:jobs"]` and nothing else. If that token leaks in logs/errors, it can't trigger plugins or access admin endpoints.

**2. Defense Against Accidents:** When you're debugging at 2am, guard rails prevent `curl -X POST /trigger/production-deploy/run` from succeeding with your monitoring token. The "I don't trust myself with a loaded gun" problem.

The implementation is straightforward (scope checking is a few string comparisons), and the config syntax is self-documenting. The cost of implementing is ~2 hours; the cost of NOT implementing is discovering a leaked token has been triggering random plugins for a week.

**Priority:** High. Essential before distributing tokens to external webhook integrations (Sprint 3). Without scopes, every webhook integration gets full access.
