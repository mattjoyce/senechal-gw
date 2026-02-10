# Task: TypeScript Bun Plugin Tutorial

## Status: todo

## Description
Create a tutorial and example plugin for using TypeScript with the Bun runtime. This will demonstrate how to build high-performance, type-safe plugins for the Senechal Gateway.

## TODO
- [ ] Create example plugin directory `plugins/ts-bun-greet`
- [ ] Implement `manifest.yaml` for the plugin
- [ ] Create `run.ts` with JSON protocol handling (stdin/stdout)
- [ ] Add Bun-specific instructions to `USER_GUIDE.md`
- [ ] Verify plugin execution with `senechal-gw run`

## Context
Bun provides a fast, all-in-one JavaScript/TypeScript runtime that is ideal for the "spawn-per-command" model of Senechal Gateway due to its low startup latency.
