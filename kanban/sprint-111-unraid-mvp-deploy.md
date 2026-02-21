# Sprint #111 — Ductile MVP Deployment to Unraid

**Type:** sprint
**ID:** 111
**Status:** doing
**Created:** 2026-02-21
**Assignee:** matt

---

## Goal

Deploy ductile as a production Docker container on Unraid B450 (192.168.20.4), building from the GitHub repo for easy updates. MVP scope: 5 Python/bash plugins, LAN-only access on port 8888, workspace volume mapped to `/mnt/user/Projects/ductile_workspace/`.

---

## Key Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Deployment method | Docker Compose | Matches all 15 existing containers |
| Container base | Alpine (Go builder) | Matches existing Dockerfile in repo |
| Source | GitHub clone (`git@github.com:mattjoyce/ductile`) | Easy updates via `git pull + rebuild` |
| Port | 8888 | 8080 taken (mattjoyce-site) |
| MVP plugins | file_handler, fabric, echo, jina-reader, youtube_transcript | All Python/bash |
| Workspace | `/mnt/user/Projects/ductile_workspace/` | Mapped as `/app/workspace` in container |
| Data path | `/mnt/user/appdata/ductile/` | Standard Unraid appdata pattern |
| Network exposure | LAN-only (192.168.20.4:8888) | cloudflared tunnel in Phase 2 |
| Log level | `info` | Production (test uses `debug`) |
| API token | Production token (set at deploy time) | Clean separation from test env |

---

## Staging Files

All deployment files staged at: `/mnt/Projects/unraid_admin/ductile/`

- `Dockerfile` — production build (Alpine + Go builder + Python plugin deps)
- `docker-compose.yml` — production compose
- `config/` — production configs (config.yaml, api.yaml, plugins.yaml, pipelines.yaml, tokens.yaml)
- `deploy.sh` — initial deploy script
- `update.sh` — update/rebuild script
- `README.md` — deploy procedure

---

## Tasks

- [ ] Scaffold staging directory and config files
- [ ] Deploy script + update script
- [ ] Create `/mnt/user/Projects/ductile_workspace/` on Unraid
- [ ] Clone repo and build on Unraid
- [ ] Set production API token
- [ ] Smoke test: healthz, echo plugin job
- [ ] Health check script (`check_ductile_status.sh`)
- [ ] Add to `run_all_healthchecks.sh`
- [ ] Vault documentation

---

## Verification

1. `docker logs ductile` — no startup errors
2. `curl http://192.168.20.4:8888/healthz` — returns 200
3. Plugin list via API returns 5 expected plugins
4. Echo job completes successfully
5. `data/ductile.db` persists across container restart
6. File written to `/mnt/user/Projects/ductile_workspace/` visible from host

---

## Phase 2 (Out of Scope)

- cloudflared tunnel exposure
- agenticloop plugin integration
- ts-bun-greet plugin (requires Bun runtime)
