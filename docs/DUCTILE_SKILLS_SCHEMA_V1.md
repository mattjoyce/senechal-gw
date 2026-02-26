# ductile.skills.schema.v1

Status: Draft  
Version: v1  
Applies to: ductile skills --json

---

## 1. Purpose

ductile.skills.schema.v1 defines the structural and semantic contract for the
"ductile skills" capability surface.

This schema guarantees deterministic, low-entropy, strongly semantically
anchored output suitable for autonomous LLM operators.

---

## 2. Design Goals

- Deterministic structure
- Explicit semantic anchors
- No inference-required safety semantics
- Stable ordering
- Versioned evolution path

---

## 3. Top-Level Entity

YAML shape:

entity: gateway
name: string
schema_version: ductile.skills.schema.v1
plugins: [plugin]
pipelines: [pipeline]
utilities: [utility]

---

## 4. Entity Definitions

### 4.1 Plugin

YAML shape:

entity: plugin
name: string
description: string
invocation:
  transport: http
  method: string
  path_template: string
commands: [command]

Required fields:
- entity
- name
- description
- invocation
- commands

---

### 4.2 Command

YAML shape:

entity: command
name: string
description: string
mutates_state: boolean
idempotent: boolean
retry_safe: boolean
input_schema: object
output_schema: object
emits_events?: [event]
execution?: execution

Required fields:
- entity
- name
- mutates_state
- idempotent
- retry_safe
- input_schema
- output_schema

---

### 4.3 Pipeline

YAML shape:

entity: pipeline
name: string
description: string
triggers: [string]
steps: [string]

---

### 4.4 Utility

YAML shape:

entity: utility
name: string
description: string
mutates_state: boolean

---

## 5. Structural Invariants

- Stable alphabetical ordering by name
- No duplicate entity names within scope
- Deterministic serialization
- No implicit safety semantics

---

## 6. Evolution Policy

Breaking structural changes require a schema version bump.
Additive fields may be introduced within v1 if optional.

