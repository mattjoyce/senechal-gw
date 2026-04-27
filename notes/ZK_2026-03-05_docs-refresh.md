# ZK Update — Documentation & "Grok-ability" Refresh (2026-03-05)

## Summary of Changes

A major pass was made over the core documentation surfaces (`README.md`, `ARCHITECTURE.md`, `GLOSSARY.md`) to elevate the project's pitch, clearly define its role in homelab automation, and synchronize the docs with recent "GT7" concurrency developments.

## Shipped to `main`

### 1. `README.md` Overhaul
- **Elevator Pitch Refined:** Repositioned Ductile explicitly as "The Glue for Your Homelab Automation," contrasting it against heavier engines like n8n and Node-RED.
- **Grokking in 30 Seconds:** Added a clear text-based flow diagram explaining the relationship between Triggers, Pipelines, Connectors, and the Queue.
- **Expanded Capabilities List:** Surfaced the strongest features to the front page:
  - Polyglot Runtime
  - Event-Driven Pipelines
  - Smart Scheduling
  - Secure Webhooks
  - Parallel Dispatch
  - Plugin Aliasing
  - Resilient Queue
  - LLM-First Discovery
- **"What Can You Build?" Examples:** Provided concrete YAML examples for three high-value use cases:
  1. The "YouTube Wisdom" Pipeline (playlist → transcript → LLM summarize → discord)
  2. The "Repo Sentinel" (webhook → policy check → alert)
  3. The "Astro Staging Rebuild" (folder watch → rebuild)

### 2. `ARCHITECTURE.md` Synchronization
- **Concurrency Accuracy:** Updated the "Execution" and "Dispatch" sections to accurately reflect the bounded worker pool, global `max_workers` cap, and per-plugin parallelism limits introduced in the GT7 rollout (replacing the outdated "serial single lane" description).
- Maintained consistency with the "Governance Hybrid" (Control vs. Data Plane) model.

### 3. `GLOSSARY.md` Expansion
- **New GT7 Terminology:** Added definitions for:
  - Worker Pool (Max Workers)
  - Parallelism
  - Concurrency Safe (manifest hint)
  - Smart Dequeue
- **Conceptual Clarity:** 
  - Clarified the distinction between "Plugin" (the code/implementation) and "Connector" (the logical integration).
  - Added "Alias (Plugin Instance)" to explain the `uses:` configuration.
  - Added "Event Bus" to describe the internal routing layer.
  - Linked "Context" explicitly to "Baggage" propagation.

## Rationale
The engine has evolved significantly with the GT7 concurrency controls, schema validations, and plugin aliasing. The documentation needed to "catch up" to these capabilities so that a new user (or LLM agent) reviewing the repo can immediately understand *what* the system does, *why* it's valuable, and *how* the modern dispatch architecture actually works.
