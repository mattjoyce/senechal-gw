---
id: 46
status: done
priority: Normal
blocked_by: []
assignee: "@claude"
tags: [tutorial, typescript, bun]
---

# Task: TypeScript Bun Plugin Tutorial

## Description
Create a tutorial and example plugin for using TypeScript with the Bun runtime. This will demonstrate how to build high-performance, type-safe plugins for the Ductile.

## Job Story
When I want to build a high-performance plugin, I want to use TypeScript and Bun, so I can have type safety and low startup latency.

## TODO
- [ ] Create example plugin directory `plugins/ts-bun-greet`
- [ ] Implement `manifest.yaml` for the plugin
- [ ] Create `run.ts` with JSON protocol handling (stdin/stdout)
- [ ] Add Bun-specific instructions to `USER_GUIDE.md`
- [ ] Verify plugin execution with `ductile run`

## Context
Bun provides a fast, all-in-one JavaScript/TypeScript runtime that is ideal for the "spawn-per-command" model of Ductile due to its low startup latency.

## Narrative
- 2026-02-10: Created Kanban card for the TypeScript Bun tutorial task. (by @gemini)
- 2026-02-11: Implemented ts-bun-greet example plugin with typed protocol v1 interfaces, poll and health commands, configurable greeting. Tested with bun runtime. (by @claude)