# Testing Guide

This document outlines the testing strategy and procedures for Ductile, including manual E2E validation and automated CLI testing.

---

## 1. Automated Tests

Ductile uses standard Go testing patterns.

### Unit & Integration Tests
Run all internal tests:
```bash
go test ./...
```

### CLI Test Suite
Comprehensive testing of CLI actions, exit codes, and output formats:
```bash
# See docs/CLI_DESIGN_PRINCIPLES.md for the standards being tested
go test ./cmd/ductile/...
```

---

## 2. Manual E2E Validation (The Echo Runbook)

To verify the full "Trigger to Output" lifecycle, use the `echo` plugin.

1.  **Configure:** Set a 5-minute schedule for `echo` in `config.yaml`.
2.  **Start:** `./ductile system start`.
3.  **Verify:** Check the logs or query the state DB:
    ```bash
    sqlite3 ductile.db "SELECT * FROM plugin_state WHERE plugin_name = 'echo';"
    ```
4.  **Crash Recovery:** Force-kill the process (`kill -9`) while a job is running, then restart and verify the job is recovered.

---

## 3. Pipeline Testing

Verify multi-hop event chains using the `file-to-report` example:

1.  **Trigger:**
    ```bash
    curl -X POST http://localhost:8080/trigger/file_handler/handle 
      -H "Authorization: Bearer <token>" 
      -d '{"payload": {"action": "read", "file_path": "sample.md"}}'
    ```
2.  **Trace:** Use the `inspect` tool to view the lineage:
    ```bash
    ductile job inspect <job_id>
    ```
3.  **Artifacts:** Verify the workspace directory contains the hardlinked files from previous steps.

---

## 4. Test Environment

We provide a **Docker-based** environment for reproducible testing:
```bash
cd ductile
docker compose up --build
```
See `TEST_ENVIRONMENT.md` for details on pre-configured tokens and ports.
