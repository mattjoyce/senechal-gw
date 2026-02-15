# Ductile Gateway: LLM Operator Skill Manifest

This manifest describes the capabilities of the current Ductile instance.

## 1. Core CLI Skills
Use these via direct CLI execution. Prefer `--json` for structured data.

### config
- `config check`: Validate syntax, policy, and integrity.
- `config lock`: Authorize current state (re-generate hashes).
- `config show [entity]`: View resolved configuration.
- `config get <path>`: Read specific config values.
- `config set <path>=<val>`: Update config (use `--dry-run` first).

### system
- `system status`: Check gateway health and PID lock.
- `system reset <plugin>`: Reset a tripped circuit breaker.
- `system watch`: Real-time diagnostic TUI.

### job
- `job inspect <job_id>`: Retrieve logs and lineage for a job.

## 2. Atomic Plugin Skills
Invoke these directly via `POST /plugin/{plugin}/{command}`.
Bypasses automated pipeline routing.

### echo
**Description:** A demonstration plugin that echoes input, emits events, and demonstrates protocol v2 state merging.

**Actions:**
- `poll`: [WRITE] Emits echo.poll events and updates the internal last_run timestamp.
- `health`: [READ] Returns the current health status and plugin version.

### fabric
**Description:** Wrapper for fabric AI pattern tool - processes text through predefined patterns

**Actions:**
- `poll`: [WRITE] 
- `handle`: [WRITE] 
- `health`: [READ] 

### file_handler
**Description:** Read and write local files with path restrictions

**Actions:**
- `poll`: [WRITE] 
- `handle`: [WRITE] 
- `health`: [READ] 

### youtube_transcript
**Description:** Fetch YouTube video transcripts from URL or video ID

**Actions:**
- `poll`: [WRITE] 
- `handle`: [WRITE] 
- `health`: [READ] 

