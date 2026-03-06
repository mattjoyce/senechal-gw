# ductile-dzt docker functional results

## Environment
- Image built from repo `Dockerfile`
- Container command: `./ductile system start --config /app/config.yaml`
- Mounted config assets from `.agent-notes/docker-functional/`
- API verified at `http://127.0.0.1:18080/healthz`

## Health
- API returned 200 OK
- Reported config path: `/app/config.yaml`

## Functional test: `if=false`
Triggered pipeline `if-demo` with payload:
```json
{"run_guarded": false, "case": "false-branch"}
```
Observed:
- root job status: `succeeded`
- tree contained one job only
- guarded step job status: `skipped`
- skip reason persisted in both `result.reason` and `last_error`

Representative response excerpt:
```json
{
  "status": "succeeded",
  "result": {"reason": "if condition evaluated false", "status": "skipped"},
  "tree": [
    {
      "plugin": "sys_exec",
      "status": "skipped",
      "last_error": "if condition evaluated false"
    }
  ]
}
```

## Functional test: `if=true`
Triggered pipeline `if-demo` with payload:
```json
{"run_guarded": true, "case": "true-branch"}
```
Observed:
- root job status: `succeeded`
- tree contained two jobs
- guarded step executed successfully
- downstream `final` step also executed successfully

Representative response excerpt:
```json
{
  "status": "succeeded",
  "tree": [
    {"plugin": "sys_exec", "status": "succeeded"},
    {"plugin": "sys_exec", "status": "succeeded"}
  ]
}
```

## Important observation
For synchronous API responses, when the first guarded entry step is skipped, the returned tree contained only the skipped root job and did not include the downstream successor step, even though dispatcher skip-continuation behavior is implemented and covered in non-Docker integration tests.

That suggests a functional gap in root synchronous pipeline waiting/entry handling for skipped-first-step cases, likely in API/root-trigger orchestration rather than dispatcher-only skip logic.

## Conclusion
- Dockerized runtime confirms first-class `if` support is live
- `if=true` path works end-to-end
- `if=false` correctly records `skipped` status and reason
- There is likely a remaining bug for the root-entry skipped-step continuation path in API/synchronous execution
