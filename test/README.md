# Ductile Local Dev Test Configuration

This directory contains the configuration and plugins used for local integration testing
of ductile with AgenticLoop. It documents a working end-to-end setup.

## What's Here

```
test/
├── config/
│   ├── config.yaml      # Root config (entry point)
│   ├── api.yaml         # API server + auth tokens
│   ├── plugins.yaml     # Plugin enable/schedule/config
│   └── pipelines.yaml   # Event-driven pipelines
└── plugins/
    └── agenticloop/     # ductile plugin that wakes AgenticLoop runs
        ├── manifest.yaml
        └── run.py
```

## Setup

### Prerequisites

- ductile binary (built from this repo)
- AgenticLoop binary (built from github.com/mattjoyce/AgenticLoop)
- Python 3 (for agenticloop plugin)
- yt-dlp in PATH (for youtube_transcript plugin)
- Plugins: `echo`, `fabric`, `file_handler`, `youtube_transcript` in your `plugins_dir`

### Environment Variables

```bash
export AGENTICLOOP_API_TOKEN=<your-agenticloop-api-token>
```

The `agenticloop` plugin reads this token from its plugin config entry in `plugins.yaml`
(set `config.token` directly or use env var interpolation).

### Directory Layout

Create the following alongside your config and plugins directories:

```
your-test-dir/
├── config/          # → contents of test/config/
├── plugins/         # → your plugin dirs (echo, fabric, ..., agenticloop)
├── data/            # created at runtime (ductile.db)
├── test-files/      # input files for file_handler reads
└── reports/         # output files from file_handler writes
```

### Starting

```bash
ductile config check --config ./config/config.yaml
ductile config lock  --config ./config/config.yaml
ductile system start --config ./config/config.yaml
```

## E2E Test: ductile → AgenticLoop → youtube_transcript

With both ductile (`:8080`) and AgenticLoop (`:8090`) running:

```bash
curl -X POST http://localhost:8080/plugin/agenticloop/handle \
  -H "Authorization: Bearer test_admin_token_local" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "goal": "Fetch the transcript for https://www.youtube.com/watch?v=RtMLnCMv3do and write a summary to summary.md",
      "wake_id": "yt-summary-test-001"
    }
  }'
```

Poll the returned `job_id` via `GET /job/{job_id}` to confirm the AgenticLoop run was queued.
The agent will call `ductile_youtube_transcript_handle` via the plugin discovery API and write
`summary.md` to its workspace.

## Token

The test token `test_admin_token_local` has full `*` scope. Replace with a scoped token
for anything beyond local dev.

## Notes

- Plugin discovery endpoints (`GET /plugins`, `GET /plugin/{name}`) are used by AgenticLoop
  to expose typed tool schemas to the LLM — see `feat/api-plugin-discovery`.
- The `agenticloop` plugin in `test/plugins/` is not bundled with ductile by default; copy
  it into your `plugins_dir` alongside the other plugins.
