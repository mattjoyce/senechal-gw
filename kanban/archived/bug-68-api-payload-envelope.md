---
id: 68
status: done
priority: Normal
tags: [api, bug, fix]
---

# BUG: API handle payload missing Event envelope

Fixed by PR #25. API now wraps raw handle payloads in a protocol.Event envelope with type 'api.trigger'.
