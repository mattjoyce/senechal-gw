---
type: improvement
id: 90
status: done
created: 2026-02-13
completed: 2026-02-13
---

# Rename to Protocol 2

## Problem
Current naming is confusing:
- Code says `"protocol": 1`
- Docs refer to "Protocol v2" as future
- But "v2" features (context, workspace_dir, auto-merge) are already implemented

This creates confusion about what protocol version is actually in use.

## Solution
Clean rename from protocol 1 to protocol 2:
- Update protocol validation to require version 2
- Update all plugin manifests to declare protocol 2
- Update dispatcher to send protocol 2
- Update documentation references

## Changes Required
1. `internal/protocol/codec.go`: Change validation from `!= 1` to `!= 2`
2. `internal/plugin/discovery.go`: Change `supportedProtocol = 1` to `= 2`
3. `internal/dispatch/dispatcher.go`: Change `Protocol: 1` to `Protocol: 2`
4. All plugin manifests: Change `protocol: 1` to `protocol: 2`
5. Update comments/docs that reference "protocol v1"

## Breaking Change
This is a breaking change for external plugins - they must update their manifests to `protocol: 2`.

## Benefits
- Clear, unambiguous naming
- Aligns version number with actual feature set
- No confusion about "v1" vs "v2"
