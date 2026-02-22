# Improvement 122: Dual-UX Skills Protocol (--skills)

**Status:** TODO  
**Owner:** Gemini  
**Context:** RFC-004, RFC-005, LX (LLM Experience)

## 1. Objective
Refactor the `ductile` CLI to support a "Dual-UX" interface. Transform the CLI from a simple tool into a **Self-Describing Agentic Environment** by promoting `--skills` to a global protocol that reframes `--help` for LLM Operators.

## 2. Core Concepts
- **The Handshake:** `ductile --skills` (or `-s`) acts as the primary discovery event for an LLM to map the gateway's capabilities.
- **Token Frugality:** Unlike `--help` (which uses verbose human prose), `--skills` returns high-density, Markdown-formatted capability maps designed for token-efficient reasoning.
- **Domain-Specific Discovery:** `ductile <noun> --skills` returns only the skills relevant to that domain (e.g., `config` vs `job`).
- **Capability Tiers:** Flags commands with semantic tags: `[READ]`, `[WRITE]`, `[GOVERNANCE]`, `[CRITICAL]`.

## 3. Tasks
- [ ] **Global Flag Detection:** Implement `hasSkillsFlag` in `cmd/ductile/main.go`.
- [ ] **Modular Skill Registry:** Refactor `runSystemSkills` into a package-level utility that can be scoped by Noun.
- [ ] **Domain-Specific Implementations:**
    - [ ] `runConfigSkills`: Map integrity, locking, and path-based setting capabilities.
    - [ ] `runJobSkills`: Map lineage, logs, and observability.
    - [ ] `runSystemSkills`: Map health, life-cycle, and global state.
- [ ] **Schema Mapping:** Include YAML path schemas in `config --skills` to eliminate "path-guessing" hallucinations.

## 4. Why This is Novel (The "Alpha")
While MCP (Model Context Protocol) provides plumbing to tools, Ductile provides a **Reasoning Environment**. By baking `--skills` into the CLI hierarchy, we ensure that an LLM can surgically discover the "governance" of the system (how to change it safely) rather than just "calling a function."
