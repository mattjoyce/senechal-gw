---
id: 116
status: backlog
priority: Low
blocked_by: []
assignee: ""
tags: [plugins, python, uv, tooling, dx]
---

# #116: Uplift Existing Python Plugins to uv

Migrate the existing Python plugins from `requirements.txt` + pip to `uv` with PEP 723 inline script metadata. `uv` is now installed as a standard tool in the ductile Docker image.

## Background

`uv` was added to the ductile Dockerfile (Feb 2026) as the standard Python tool for plugin development. New plugins (e.g. ductile-withings) will use `uv run` with inline script metadata from the start. Existing plugins should be uplifted for consistency and to remove the separate `requirements.txt` dependency install step from the image build.

## Plugins to Uplift

- [ ] `plugins/fabric/` — currently `run.py` + `requirements.txt`
- [ ] `plugins/file_handler/` — currently `run.py` + `requirements.txt`
- [ ] `plugins/jina-reader/` — currently `run.py` + `requirements.txt`
- [ ] `plugins/youtube_transcript/` — currently `run.py` + `requirements.txt`

`plugins/echo/` (bash) and `plugins/ts-bun-greet/` (TypeScript) are not affected.

## Migration Pattern

For each plugin, replace `requirements.txt` + bare `python3 run.py` entrypoint with inline PEP 723 metadata and `uv run`:

**Before:**
```
plugins/fabric/
├── manifest.yaml   (entrypoint: run.py)
├── requirements.txt
└── run.py
```

**After:**
```
plugins/fabric/
├── manifest.yaml   (entrypoint: run.py)
└── run.py          (with inline deps, invoked via `uv run run.py`)
```

`run.py` header:
```python
# /// script
# dependencies = [
#   "requests>=2.31",
# ]
# ///
```

`manifest.yaml` entrypoint change:
```yaml
entrypoint: run.py   # ductile invokes via `uv run run.py` if uv present
```

> Note: Confirm how ductile dispatcher invokes the entrypoint — may need a thin `run.sh` wrapper calling `uv run run.py`, or native uv support in the dispatcher.

## Dockerfile Cleanup

Once all plugins are migrated, remove the build-time pip install step:

```dockerfile
# Remove this:
RUN find /app/plugins -name requirements.txt \
    -exec pip3 install --no-cache-dir -r {} \;
```

Dependencies are then resolved at runtime by `uv` on first invocation (cached in `UV_CACHE_DIR`).

## Acceptance Criteria

- [ ] All four Python plugins run correctly under `uv run`
- [ ] No `requirements.txt` files remain in plugin directories
- [ ] Dockerfile no longer has the `find requirements.txt` install step
- [ ] Plugin startup time is acceptable (uv cache warm on second run)

## Narrative

- 2026-02-21: Card created. `uv` added to Dockerfile as standard tooling. New plugins (ductile-withings) will use uv from the start; existing plugins to be uplifted as a separate effort.
