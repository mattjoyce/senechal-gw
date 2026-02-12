---
id: 89
status: todo
priority: Medium
tags: [docs, config, auth, ux]
---

# DOC: Auth configuration structure unclear (api_key vs tokens)

## Description

The authentication configuration supports both legacy `api_key` (single token) and new `tokens` array (scoped tokens), but docs don't clearly explain the difference or migration path.

## Impact

- **Severity**: Medium - Auth works but setup is confusing
- **Scope**: New deployments, config migration
- **User Experience**: Trial and error to get auth working

## Evidence

**Test**: TP-002 Setup (2026-02-13)

**Attempts made**:

1. **Top-level tokens** (failed):
   ```yaml
   tokens:
     - name: admin
       key: test_token
       scopes: ["*"]
   ```

2. **api.auth.tokens only** (failed):
   ```yaml
   api:
     auth:
       tokens:
         - token: test_token
           scopes: ["*"]
   ```

3. **api.auth.api_key** (worked):
   ```yaml
   api:
     auth:
       api_key: test_token  # Legacy field
       tokens:
         - token: test_token
           scopes: ["*"]
   ```

## Root Cause

**Code**: Multiple auth mechanisms coexist

`/home/matt/senechal-gw/internal/api/server.go`:
```go
type Config struct {
    // APIKey is the legacy single bearer token (admin/full access).
    APIKey string
    // Tokens is an optional list of scoped bearer tokens.
    Tokens []auth.TokenConfig
}
```

**Auth middleware** (`auth.go`):
```go
principal, ok := auth.Authenticate(token, s.config.APIKey, s.config.Tokens)
```

**What's unclear**:
- Which field to use when?
- Can both coexist?
- Migration path from api_key to tokens?
- What happens if only tokens defined?

## Recommendation

### 1. Update CONFIG_SPEC.md

Add clear auth section:

```markdown
## Authentication Configuration

Senechal supports two authentication modes:

### Legacy: Single API Key (Simple)

For single-user or development:

```yaml
api:
  enabled: true
  auth:
    api_key: your_secret_token_here
```

**Use this when**: You need quick setup with full admin access.

### Modern: Scoped Tokens (Recommended)

For multi-user or production:

```yaml
api:
  enabled: true
  auth:
    tokens:
      - token: admin_token
        scopes: ["*"]
      - token: readonly_token
        scopes: ["plugin:ro"]
```

**Use this when**: You need granular permissions.

### Migration Path

1. Add both during transition:
   ```yaml
   api:
     auth:
       api_key: old_token      # Legacy for existing integrations
       tokens:
         - token: new_token_1
           scopes: ["*"]
   ```

2. Migrate clients to new tokens

3. Remove api_key field

### Scopes

- `*` - Full access (admin)
- `plugin:ro` - Read-only plugin operations (poll, health)
- `plugin:rw` - Read-write plugin operations (handle, trigger)
```

### 2. Add validation warning

If api_key used, log:
```
WARN: Using legacy api_key. Consider migrating to scoped tokens.
```

### 3. Update USER_GUIDE.md

Show both approaches with clear recommendations.

## Testing Impact

During TP-002 setup, spent 20+ minutes debugging auth failures trying different config structures. Eventually found api_key worked via code inspection, not documentation.

## Related

- CONFIG_SPEC.md: Needs auth section
- USER_GUIDE.md: Example uses api_key but doesn't explain it
- Bug #84: Database path ignored (similar config documentation gap)

## Narrative

- 2026-02-13: Discovered during TP-002 auth setup. Initially tried tokens array following CONFIG_SPEC pattern for high-security files, but authentication failed with "invalid API key". Tried various configurations: top-level tokens, api.auth.tokens, api.auth.api_key. Only api_key field worked. Code inspection revealed dual auth system (legacy api_key + new scoped tokens) but docs don't explain which to use or how they interact. This caused significant troubleshooting time. Recommend clear documentation of both modes with migration guidance. (by @test-admin)
