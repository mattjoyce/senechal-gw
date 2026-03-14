# ductile-dzt docker functional test spec

## Goal
Functionally verify first-class structured `if` step gating in a Dockerized Ductile runtime using the repo Dockerfile.

## Files to touch
- `Dockerfile` (read only for test setup understanding)
- temporary test assets under `.agent-notes/docker-functional/`
  - `config.yaml`
  - `pipelines.yaml`
  - `plugins.yaml`
  - `.env.test`
- no product code changes planned for this testing step

## Expected behaviour
- Container starts successfully from the repo Dockerfile
- API is reachable
- Triggering a pipeline with `if` false results in:
  - first step job status `skipped`
  - downstream step still executes successfully
- Triggering same pipeline with `if` true results in:
  - guarded step executes successfully
  - downstream step executes successfully
- Observable result available through API `/jobs/:id`

## Edge cases to observe
- config include path resolution inside container
- plugin/runtime dependencies present in runtime image
- skipped step appears in returned job tree with status `skipped`
- no plugin spawn needed for false branch gate

## Verification plan
1. Build image with Dockerfile
2. Run container with mounted temp config and plugins
3. Wait for API health/reachability
4. Trigger `if=false` and inspect returned tree/job data
5. Trigger `if=true` and inspect returned tree/job data
6. Capture concise evidence in notes
