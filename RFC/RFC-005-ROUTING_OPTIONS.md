# RFC-005: Routing Payload Management Strategies

**Status:** Draft (partially implemented; pending final consolidation)  
**Date:** 2026-02-11  
**Author:** Matt Joyce  
**Depends on:** RFC-001, RFC-002  
**Supersedes:** None (net new concern)

---

## Problem Statement

RFC-002 Decision 15 defines routing semantics (fan-out, exact match, no filters) but doesn't specify **how payloads flow through multi-hop event chains**.

### Concrete Example

Discord user: "extract wisdom from <video_url>"
```
[discord-handler] emits transcript_requested
    ↓
[transcript-downloader] emits transcript_ready  
    ↓
[wisdom-extractor] emits wisdom_extracted
    ↓
[document-publisher] emits document_published
    ↓
[discord-notifier] needs to post result back to original Discord channel
```

**The question:** How does `discord-notifier` (job-005) know which Discord channel to notify?

### Why This Matters

- **Multiple initiators**: Discord, email, manual CLI, Slack webhooks, scheduled polls
- **Long chains**: 5+ hops through different plugins
- **Large payloads**: Video transcripts (100KB+), images, documents
- **Fan-out**: One event triggers multiple independent chains
- **Context loss**: Intermediate plugins don't care about initiator context

---

## Option 1: Baggage Forwarding (Explicit Pass-Through)

### Mechanism

Every plugin **explicitly includes** upstream context in its event payloads.
```python
# discord-handler (hop 1)
response = {
    "events": [{
        "type": "transcript_requested",
        "payload": {
            "video_url": "https://youtube.com/...",
            "channel_id": "discord-123",      # Initiator context
            "user_id": "user-456"
        }
    }]
}

# transcript-downloader (hop 2)
# Must remember to pass context forward:
response = {
    "events": [{
        "type": "transcript_ready",
        "payload": {
            "transcript": "...",
            "channel_id": event["channel_id"],    # ← Forwarded
            "user_id": event["user_id"]           # ← Forwarded
        }
    }]
}

# wisdom-extractor (hop 3)
response = {
    "events": [{
        "type": "wisdom_extracted",
        "payload": {
            "wisdom": "...",
            "channel_id": event["channel_id"],    # ← Still forwarding
            "user_id": event["user_id"]
        }
    }]
}

# ... and so on
```

### Core Properties

**Contract:**
- Plugin authors manually propagate context they don't use
- No schema enforcement (pure convention)
- Payload grows as it passes through chain

**Data flow:**
```
job-001: {"channel_id": "X", "video_url": "Y"}
job-002: {"channel_id": "X", "video_url": "Y", "transcript": "Z"}
job-003: {"channel_id": "X", "video_url": "Y", "transcript": "Z", "wisdom": "W"}
job-004: {"channel_id": "X", "video_url": "Y", "transcript": "Z", "wisdom": "W", "doc_url": "D"}
```

**Storage:**
- Only in `job_queue.payload` (JSON column)
- No separate tracking table

### Advantages

1. **Loose coupling**: No database dependency for payload flow
2. **Plugin autonomy**: Each plugin sees full context, decides what to include
3. **Explicit**: Data flow visible in plugin code
4. **Simple GW logic**: Router just passes payload as-is
5. **Stateless plugins**: No DB queries needed
6. **Easy debugging**: Full context in every job log

### Disadvantages

1. **Brittle conventions**: Plugin forgets to forward → chain breaks silently
2. **Payload bloat**: Transcript (100KB) carried through all downstream jobs
3. **No deduplication**: Same large data copied 5 times in DB
4. **Typo risk**: `channeI_id` vs `channel_id` breaks routing
5. **Plugin burden**: Every plugin author must understand forwarding contract
6. **No history**: Can't query "what context existed at hop 3?"

### Failure Modes

**Silent context loss:**
```python
# wisdom-extractor forgets to forward channel_id
response = {
    "events": [{
        "type": "wisdom_extracted",
        "payload": {
            "wisdom": "..."
            # Oops, no channel_id!
        }
    }]
}
# discord-notifier fails: KeyError('channel_id')
```

**Payload bloat with large data:**
```sql
-- job_queue table
job-002: payload = 120 KB (includes transcript)
job-003: payload = 125 KB (transcript + wisdom)
job-004: payload = 126 KB (transcript + wisdom + url)
-- 371 KB stored, mostly duplicated transcript
```

---

## Option 2: Database-Mediated Context (Accumulated Payloads)

### Mechanism

Gateway maintains an `event_context` table that accumulates payloads across the chain. Plugins only provide **incremental** data.

**New schema:**
```sql
event_context (
  event_id        TEXT PRIMARY KEY,     -- UUID
  parent_event_id TEXT,                 -- FK to parent event
  job_id          TEXT NOT NULL,
  plugin          TEXT NOT NULL,
  event_type      TEXT NOT NULL,
  payload         JSON NOT NULL,        -- Incremental (plugin's contribution)
  accumulated     JSON NOT NULL,        -- Full merged context
  created_at      TEXT NOT NULL
);

job_queue (
  -- ... existing fields ...
  event_context_id TEXT,                -- FK to event_context
  payload          JSON                 -- DEPRECATED
);
```

**Plugin behavior:**
```python
# discord-handler (hop 1)
response = {
    "events": [{
        "type": "transcript_requested",
        "payload": {
            "video_url": "https://youtube.com/..."
            # Does NOT include channel_id
        }
    }]
}

# GW stores context:
INSERT INTO event_context (event_id, payload, accumulated) VALUES (
    'event-001',
    '{"video_url": "..."}',                                      -- Incremental
    '{"video_url": "...", "channel_id": "X", "source": "discord"}' -- Full
);

# transcript-downloader (hop 2)
response = {
    "events": [{
        "type": "transcript_ready",
        "payload": {
            "transcript": "..."
            # Does NOT forward channel_id
        }
    }]
}

# GW merges:
INSERT INTO event_context (event_id, parent_event_id, payload, accumulated) VALUES (
    'event-002',
    'event-001',
    '{"transcript": "..."}',                                     -- Just new data
    '{"video_url": "...", "channel_id": "X", "transcript": "..."}' -- Inherited + new
);
```

**Plugin receives:**
```json
{
  "protocol": 2,
  "job_id": "job-005",
  "command": "handle",
  "event_context": {
    "event_id": "event-005",
    "accumulated": {
      "video_url": "...",
      "channel_id": "discord-123",
      "transcript": "...",
      "wisdom": "...",
      "doc_url": "..."
    }
  }
}
```

### Core Properties

**Contract:**
- GW manages context accumulation
- Plugins only emit their contribution
- Full context always available via event_context_id

**Data flow:**
```
event-001: payload={"video_url": "Y"},  accumulated={"channel_id": "X", "video_url": "Y"}
event-002: payload={"transcript": "Z"}, accumulated={"channel_id": "X", "video_url": "Y", "transcript": "Z"}
event-003: payload={"wisdom": "W"},     accumulated={"channel_id": "X", "video_url": "Y", "transcript": "Z", "wisdom": "W"}
```

**Storage:**
- `event_context` table with full history
- `job_queue` only references event_context_id

### Advantages

1. **Unbreakable chain**: Plugin can't forget to forward context
2. **Queryable history**: SQL can reconstruct full chain evolution
3. **Deduplication**: Large payloads stored once per hop, not per job
4. **Explicit merge semantics**: GW controls key conflicts (last-write-wins)
5. **Plugin simplicity**: Authors don't think about forwarding
6. **Traceability**: `parent_event_id` forms explicit chain

### Disadvantages

1. **Database dependency**: Payload flow requires DB reads/writes
2. **GW complexity**: Router must implement merge logic
3. **Coupling to storage**: Can't route without DB
4. **Schema migration**: New table, modified job_queue
5. **Performance**: Extra DB roundtrip per routing hop
6. **Large accumulated payloads**: 1MB limit still applies, just in `accumulated` column

### Failure Modes

**Key collision:**
```python
# Plugin A emits: {"status": "processing"}
# Plugin B emits: {"status": "complete"}
# Last-write-wins: accumulated["status"] = "complete"
# Plugin A's status is lost
```

**Accumulated payload explosion:**
```sql
-- event-003 accumulated: 900 KB (video_url + transcript + wisdom)
-- event-004 tries to add 200 KB doc → exceeds 1MB limit
ERROR: Accumulated payload exceeds 1MB
```

---

## Option 3: Hybrid (Context References + Inline Data)

### Mechanism

Small, stable context (channel_id, user_id) lives in `event_context`. Large, transient data (transcripts) stays in payloads and is **not** accumulated.

**Schema:**
```sql
event_context (
  event_id        TEXT PRIMARY KEY,
  parent_event_id TEXT,
  job_id          TEXT NOT NULL,
  context         JSON NOT NULL,        -- Small, stable keys only
  created_at      TEXT NOT NULL
);

job_queue (
  -- ... existing fields ...
  event_context_id TEXT,                -- FK for stable context
  payload          JSON                 -- Large, transient data
);
```

**Plugin behavior:**
```python
# discord-handler (hop 1)
response = {
    "context_keys": ["channel_id", "user_id", "source"],  # NEW: Declare stable keys
    "events": [{
        "type": "transcript_requested",
        "payload": {
            "video_url": "..."
            # No channel_id in payload
        }
    }]
}

# GW extracts stable context:
INSERT INTO event_context (event_id, context) VALUES (
    'event-001',
    '{"channel_id": "X", "user_id": "Y", "source": "discord"}'
);

# transcript-downloader (hop 2)
response = {
    "events": [{
        "type": "transcript_ready",
        "payload": {
            "transcript": "...(100KB)..."  # Large data stays in payload
        }
    }]
}

# GW does NOT accumulate transcript
INSERT INTO event_context (event_id, parent_event_id, context) VALUES (
    'event-002',
    'event-001',
    '{"channel_id": "X", "user_id": "Y", "source": "discord"}'  # Inherited, no transcript
);
```

**Plugin receives:**
```json
{
  "job_id": "job-003",
  "command": "handle",
  "context": {
    "channel_id": "X",
    "user_id": "Y",
    "source": "discord"
  },
  "event": {
    "transcript": "...(100KB)..."
  }
}
```

### Core Properties

**Contract:**
- Plugin declares which keys are "context" (stable, small)
- Context is accumulated, payloads are not
- Plugin receives: context (inherited) + event (immediate parent's payload)

**Data flow:**
```
event-001: context={"channel_id": "X"}, payload={"video_url": "Y"}
event-002: context={"channel_id": "X"}, payload={"transcript": "Z"}  (100KB)
event-003: context={"channel_id": "X"}, payload={"wisdom": "W"}      (50KB)
```

### Advantages

1. **Best of both**: Stable context unbreakable, large data not duplicated
2. **Size management**: Context capped at ~10KB, payloads can be large
3. **Explicit contract**: `context_keys` declares what's stable
4. **Selective accumulation**: Only small, routing-critical data accumulated
5. **Plugin clarity**: Authors know context vs. data distinction

### Disadvantages

1. **Complexity**: Two channels (context + payload)
2. **Convention burden**: Plugin authors must categorize keys
3. **Partial DB dependency**: Context in DB, payload in job record
4. **Unclear line**: Is `video_url` context or data?
5. **Protocol v2**: Breaking change from current design

---

## Option 4: External Storage References (S3/Filesystem)

### Mechanism

Large payloads stored externally. Events pass **references**, not content.
```python
# transcript-downloader
transcript = fetch_transcript(video_url)

# Store large data externally
ref = storage.put(f"transcripts/{job_id}.txt", transcript)

response = {
    "events": [{
        "type": "transcript_ready",
        "payload": {
            "transcript_ref": f"s3://bucket/transcripts/{job_id}.txt",
            "transcript_length": 150000,
            "channel_id": event["channel_id"]
        }
    }]
}

# wisdom-extractor
transcript_ref = event["transcript_ref"]
transcript = storage.get(transcript_ref)
```

### Advantages

1. **Unbounded payload size**: Transcripts, videos, images
2. **DB stays small**: job_queue.payload < 10KB
3. **Efficient storage**: No duplication in DB

### Disadvantages

1. **External dependency**: Requires S3/MinIO/NFS
2. **Lifecycle management**: When to delete stored files?
3. **Plugin complexity**: Every plugin needs storage client
4. **Failure modes**: Broken references, storage outages
5. **Not for MVP**: Over-engineered for personal gateway

---

## Comparison Matrix

| Dimension | Baggage Forward | DB Accumulated | Hybrid | External Refs |
|-----------|----------------|----------------|--------|---------------|
| **Coupling** | Loose (no DB) | Tight (DB required) | Medium | Tight (storage) |
| **Plugin burden** | High (manual forward) | Low (auto-merge) | Medium (categorize) | High (storage API) |
| **Failure risk** | High (forget forward) | Low (unbreakable) | Low | Medium (ref breaks) |
| **Payload size** | Duplicated | Accumulated | Context only | Refs only |
| **GW complexity** | Low | High (merge logic) | High | Medium |
| **Debuggability** | High (inline) | Medium (query chain) | Medium | Low (external) |
| **Schema changes** | None | New table | New table | None |
| **MVP viable** | ✅ Yes | ⚠️ Complex | ⚠️ Complex | ❌ Over-eng |

---

## Recommendation

### For MVP (Now): **Option 1 - Baggage Forwarding**

**Rationale:**
1. **Ship quickly**: No schema changes, no new tables
2. **Learn from reality**: Real workflows will reveal which keys need forwarding
3. **Plugin autonomy**: Preserves loose coupling you prefer
4. **Explicit is better**: Data flow visible in plugin code

**Mitigations for disadvantages:**
- **Document convention**: Provide template plugins with clear forwarding patterns
- **Lint helpers**: CLI tool to validate events include required keys
- **Size caps**: Reject event payloads >100KB (forces external storage thinking)

**Example pattern to document:**
```python
# Standard forwarding pattern (copy-paste template)
def forward_context(event, new_data):
    """Merge new data with forwarded context"""
    return {
        **event,      # Forward everything from upstream
        **new_data    # Add this plugin's contribution
    }

response = {
    "events": [{
        "type": "my_event",
        "payload": forward_context(event, {
            "my_new_key": "my_new_value"
        })
    }]
}
```

### Post-MVP Pivot Point (Month 2-3): **Option 3 - Hybrid**

**When to implement:**
- You have 3+ real workflows in production
- You've identified which keys are "context" vs "data"
- Baggage forwarding failures have occurred

**Migration path:**
1. Add `event_context` table (non-breaking)
2. Populate from existing payloads
3. Update router to use hybrid mode
4. Plugins opt-in to protocol v2

---

## Open Questions

1. **Large payload strategy**: If transcript is 500KB, should it be in payload at all? Or force external storage pattern from day 1?

2. **Context schema**: Should there be a standard schema for context keys (source, initiator_id, initiator_type)? Or pure convention?

3. **Merge conflicts**: In Option 2/3, if two plugins emit same key, what wins? Last-write? Error? Namespace by plugin?

4. **Fan-out handling**: With Option 2, if event-001 fans out to 3 plugins, do they all inherit same accumulated context? Or branch?

5. **Partial forwarding**: With Option 1, is it acceptable for a plugin to only forward some keys (e.g., drop transcript but keep channel_id)?

---

## Decision Criteria

**Choose Option 1 if:**
- You value loose coupling > convenience
- MVP speed is priority
- You're okay with plugin author discipline
- You expect short chains (3-5 hops max)

**Choose Option 2 if:**
- You want guaranteed context propagation
- You expect complex chains (7+ hops)
- You value queryable history
- You're okay with DB coupling

**Choose Option 3 if:**
- You have mixed payload sizes (small context + large data)
- You want explicit context vs. data separation
- You're willing to do protocol v2

**Choose Option 4 if:**
- You routinely handle >1MB payloads
- You need to share data between jobs
- You have external storage infrastructure

---

## Feedback Requested

1. Does "baggage forwarding" align with your "loose coupling" preference?
2. Are you comfortable with plugin author discipline (manual forwarding)?
3. At what chain length would you want automatic accumulation?
4. Should large payloads (>100KB) be forbidden in events, forcing external storage?
5. Is there a hybrid you'd prefer that isn't covered here?

---
