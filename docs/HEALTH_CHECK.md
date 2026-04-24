# Ductile Health Check Procedure

Operational procedure for reviewing the day-to-day health of a running Ductile instance.
Useful as a daily status review or as the first thing to run when investigating flaky behaviour.

Assumes a systemd-managed deployment. Adjust service name, binary path, and config path to
match your instance.

## Step 1 — Service & binary sanity

```bash
systemctl --user is-active <service-name>
systemctl --user status <service-name> --no-pager | sed -n '1,8p'
ductile version
ductile system status
```

Expected: `active`. Record the `version`, `commit`, and uptime — these let you correlate
errors against deploys later.

**Known quirk:** when the daemon is running, `ductile system status` reports `DEGRADED`
with `pid_lock: FAIL (pid <N>)`. The PID reported IS the running daemon. This is expected
when calling `system status` against a live instance — not a real failure. Verify the PID
matches the systemd `Main PID` and move on.

## Step 2 — Recent errors

Scan a 24h window for error-level log entries, and separately scan everything since the
current binary was started, so that pre-deploy and post-deploy issues are distinguishable.

```bash
# 24h error scan
journalctl --user -u <service-name> --since "24 hours ago" --no-pager \
  | grep -i -E '"level":"ERROR"|panic|FATAL'

# Since service restart (use the "Active: since" timestamp from `systemctl status`)
journalctl --user -u <service-name> --since "<HH:MM:SS>" --no-pager \
  | grep -i -E '"level":"ERROR"|panic|FATAL'
```

Common patterns and their meaning:

| Pattern | Meaning |
|---|---|
| `baggage path "ductile.route_depth" is immutable` | Routing/baggage propagation issue. The plugin itself may have succeeded; failure is in routed-context creation downstream. |
| `plugin fingerprint check failed (strict mode)` | A plugin entrypoint was edited without `ductile config lock`. Review the change, then re-lock. |
| `failed to create event context for pipeline entry` | Usually a symptom of a baggage/routing bug — investigate the underlying cause rather than the symptom. |

## Step 3 — 24h job stats per scheduled plugin

Enumerate scheduled plugins from the live config:

```bash
ductile config show | grep -B1 -A2 'schedules:'
```

Then collect per-plugin 24h counts:

```bash
FROM=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)
for plugin in <plugin-a> <plugin-b> <plugin-c>; do
  ductile job logs --from "$FROM" --plugin "$plugin" --limit 200 --json 2>/dev/null \
    | python3 -c "
import sys,json
d=json.load(sys.stdin)
logs=d.get('logs') or []
total=d.get('total',0)
succ=sum(1 for j in logs if j.get('Status')=='succeeded')
fail=sum(1 for j in logs if j.get('Status')=='failed')
print(f'{\"$plugin\":<22} total={total:<4} in_window={len(logs)} succ={succ:<4} fail={fail}')
"
done
```

Also query any event-driven plugins you care about; they may show 0 in the window, which is
fine if no upstream event triggered them.

**CLI gotchas:**
- JSON field names are **capitalized**: `Status`, `Plugin`, `CreatedAt`, `LastError`,
  `Stderr`, `Result`.
- `--limit` maxes at 200. When `total > in_window`, you are seeing only the most recent 200
  entries, but `total` is the truthful 24h count.

## Step 4 — Investigate failures

For any plugin showing fails, pull full details including `Result`, `LastError`, and
`Stderr` via `--include-result`:

```bash
FROM=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)
ductile job logs --from "$FROM" --plugin <name> --limit 200 --include-result --json 2>/dev/null \
  | python3 -c "
import sys,json
d=json.load(sys.stdin)
for j in d.get('logs',[]):
  if j.get('Status')=='failed':
    print(f\"  {j.get('CreatedAt')}  cmd={j.get('Command')} attempt={j.get('Attempt')}\")
    for k in ('LastError','Stderr'):
      v=j.get(k) or ''
      if v: print(f'    {k}:', v[:300])
    res=j.get('Result')
    if res: print('    Result:', json.dumps(res)[:300])
"
```

Watch for cases where `Result.status == "ok"` but `LastError` is set: the plugin itself
succeeded, and the failure is downstream in Ductile's routing/context layer. Those are
core bugs, not plugin bugs, and usually need to be matched against recent upstream commits.

For job lineage (baggage, workspace, attempt history) across a pipeline:

```bash
ductile job inspect <job-id>
```

## Step 5 — Deploy correlation

If errors cluster before a timestamp and stop after it, confirm a deploy/restart explains
it rather than transient recovery:

```bash
# find service restarts
journalctl --user -u <service-name> --since "24 hours ago" --no-pager \
  | grep -E 'Started|ductile running'

# binary age
ls -la $(command -v ductile)
```

Match against `git log` in the ductile source tree between the old and new `commit:` values
(from `ductile version`) to identify which commits fixed which errors.

## Step 6 — Verdict

Summarise as:

1. **Service state** — active/degraded (ignoring the `pid_lock` quirk), binary version, uptime.
2. **24h job totals** — overall success rate, per-plugin failure counts.
3. **Failures** — root cause(s), whether already patched in the running binary, whether
   operator action is needed.
4. **Post-restart window** — clean or not (most important signal for "is it healthy *now*?").

Target: the post-restart window has zero errors. Pre-restart errors with a matching fix
already in the running binary are history, not present-day problems.

## Related docs

- [Deployment](DEPLOYMENT.md)
- [Operator Guide](OPERATOR_GUIDE.md)
- [Architecture](ARCHITECTURE.md)
