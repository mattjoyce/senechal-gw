---
id: 31
status: done
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-2, documentation, api]
---

# Document API Endpoints in User Guide

Add documentation for Sprint 2 API endpoints (POST /trigger, GET /job) to the USER_GUIDE.md.

## Context

Sprint 2 PRs (#8, #10) added HTTP API endpoints for external triggers, but the user guide currently only documents Sprint 1 MVP features (scheduler, plugins, state, crash recovery). Need to add a new section covering API usage.

## Acceptance Criteria

- Add new section to `docs/USER_GUIDE.md` documenting API endpoints
- Section should come after "Getting Started" and before or after "Core Concepts"
- Document both endpoints with examples:
  - POST /trigger/{plugin}/{command}
  - GET /job/{job_id}
- Include authentication (Bearer token)
- Show curl examples for common scenarios
- Document configuration (`api.enabled`, `api.listen`, `api.auth.api_key`)
- Explain use case: "External triggers for LLM agents and scripts"
- 300-500 words

## Suggested Section Structure

### API Endpoints (New Section)

**Introduction:**
- What the API is for (external triggers, LLM integration)
- When to use API vs scheduler (on-demand vs periodic)

**Configuration:**
```yaml
api:
  enabled: true
  listen: "localhost:8080"
  auth:
    api_key: ${API_KEY}
```

**POST /trigger/{plugin}/{command}:**
- Purpose: Enqueue job on-demand
- Authentication: Bearer token required
- Request/response format
- Example: Trigger echo plugin

```bash
curl -X POST http://localhost:8080/trigger/echo/poll \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}'
```

**GET /job/{job_id}:**
- Purpose: Check job status and retrieve results
- Authentication: Bearer token required
- Response format (queued, running, completed with result)
- Example: Poll for job completion

```bash
JOB_ID="..."
curl http://localhost:8080/job/$JOB_ID \
  -H "Authorization: Bearer $API_KEY"
```

**Use Cases:**
- LLM tool calling ("check my calendar", "sync health data")
- External automation scripts
- Manual testing with curl
- Webhook-style triggers from other services

**Security Notes:**
- API key required for all requests
- Use environment variables for secrets
- localhost-only by default (use reverse proxy for external access)

## Implementation

Update `docs/USER_GUIDE.md`:
- Add section after "Getting Started" (section 2.5 or 3)
- Or create new top-level section 3 "Using the API" and shift others down
- Keep consistent with existing style (clear, beginner-friendly)
- Use real examples (not foo/bar)

## Verification

- Read through new section for clarity
- Test curl examples actually work
- Verify formatting consistent with rest of guide
- Check for typos

## Branch

`gemini/document-api` (or update existing `gemini/user-guide` branch)

## Deliverable

Updated `docs/USER_GUIDE.md` with API endpoints section (~300-500 words added)

## Narrative

- 2026-02-09: Created to track API documentation after PRs #8 and #10 merged. Completed as part of PR #9 - Gemini added comprehensive API section (section 3) covering POST /trigger, GET /job, authentication, configuration, use cases, and security notes. ~500 words added. Merged with user guide. (by @claude)