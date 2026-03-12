#!/usr/bin/env python3
"""sys_exec plugin (protocol v2).

COPY/FORK NOTES:
- Copy this directory and rename manifest `name:` for each custom executor.
- Configure the command in config (`plugins.<name>.config.command`).
- Payload values are exposed only as env vars, never interpolated into command text.
"""

from __future__ import annotations

import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from typing import Any, Dict, List, Tuple

SAFE_ENV_KEYS = {
    "PATH",
    "HOME",
    "USER",
    "LOGNAME",
    "SHELL",
    "TMPDIR",
    "LANG",
    "LC_ALL",
    "SSH_AUTH_SOCK",
}
DEFAULT_STDOUT_MAX = 16_384
DEFAULT_STDERR_MAX = 16_384
ENV_PREFIX = "DUCTILE_PAYLOAD_"
ENV_KEY_RE = re.compile(r"[^A-Za-z0-9_]")


def iso_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def coerce_bool(value: Any, default: bool) -> bool:
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered in {"1", "true", "yes", "on"}:
            return True
        if lowered in {"0", "false", "no", "off"}:
            return False
    return default


def coerce_int(value: Any, default: int, minimum: int = 1) -> int:
    if value is None:
        return default
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        return default
    return max(minimum, parsed)


def coerce_float(value: Any, default: float, minimum: float = 0.0) -> float:
    if value is None:
        return default
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return default
    if parsed < minimum:
        return minimum
    return parsed


def payload_value_to_env(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, (str, int, float, bool)):
        return str(value)
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False)


def env_key_for_payload_key(key: str) -> str:
    key = key.strip().upper()
    key = ENV_KEY_RE.sub("_", key)
    key = re.sub(r"_+", "_", key).strip("_")
    if not key:
        key = "VALUE"
    if key[0].isdigit():
        key = "_" + key
    return ENV_PREFIX + key


def build_payload_env(payload: Dict[str, Any]) -> Dict[str, str]:
    out: Dict[str, str] = {}
    collisions: Dict[str, int] = {}
    for raw_key, raw_value in payload.items():
        env_key = env_key_for_payload_key(str(raw_key))
        if env_key in out:
            collisions[env_key] = collisions.get(env_key, 1) + 1
            env_key = f"{env_key}_{collisions[env_key]}"
        out[env_key] = payload_value_to_env(raw_value)
    return out


def truncate_text(text: str, max_bytes: int) -> Tuple[str, bool]:
    encoded = text.encode("utf-8", errors="replace")
    if len(encoded) <= max_bytes:
        return text, False
    truncated = encoded[:max_bytes].decode("utf-8", errors="ignore")
    return truncated, True


def plugin_error(message: str, *, retry: bool = False, logs: List[Dict[str, str]] | None = None) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": logs or [{"level": "error", "message": message}],
    }


def plugin_ok(*, result: str, events: List[Dict[str, Any]] | None = None, logs: List[Dict[str, str]] | None = None, state_updates: Dict[str, Any] | None = None) -> Dict[str, Any]:
    response: Dict[str, Any] = {"status": "ok", "result": result, "logs": logs or []}
    if events:
        response["events"] = events
    if state_updates:
        response["state_updates"] = state_updates
    return response


def expand_env_vars(args: List[str], env: Dict[str, str]) -> List[str]:
    """Manually expand environment variables in the arguments list.
    Supports $VAR and ${VAR} syntax.
    """
    out = []
    for arg in args:
        # We use a regex to find $VAR or ${VAR}
        # This is a simplified expansion logic.
        def replacer(match):
            var_name = match.group(1) or match.group(2)
            return env.get(var_name, match.group(0))

        expanded = re.sub(r"\$(?:([A-Za-z0-9_]+)|\{([A-Za-z0-9_]+)\})", replacer, arg)
        out.append(expanded)
    return out


def handle_health(config: Dict[str, Any]) -> Dict[str, Any]:
    command_raw = config.get("command")
    if not command_raw:
        return plugin_error("config.command is required", retry=False)

    if isinstance(command_raw, list):
        if not command_raw:
            return plugin_error("config.command list cannot be empty", retry=False)
        command_args = [str(arg) for arg in command_raw]
    else:
        command_args = shlex.split(str(command_raw))

    if not command_args:
        return plugin_error("config.command must not be empty", retry=False)

    executable = command_args[0]
    if shutil.which(executable) is None:
        return plugin_error(f"executable not found in PATH: {executable}", retry=False)

    working_dir = str(config.get("working_dir", "")).strip()
    if working_dir:
        real = os.path.realpath(os.path.expanduser(working_dir))
        if not os.path.isdir(real):
            return plugin_error(f"working_dir is not a directory: {real}", retry=False)

    return plugin_ok(
        result="sys_exec health check passed",
        logs=[
            {"level": "info", "message": "sys_exec health check passed"},
            {"level": "debug", "message": f"configured command: {' '.join(command_args)}"},
        ],
    )


def resolve_working_dir(config: Dict[str, Any], workspace_dir: str) -> str | None:
    configured = str(config.get("working_dir", "")).strip()
    if configured:
        return os.path.realpath(os.path.expanduser(configured))
    if workspace_dir:
        return workspace_dir
    return None


def build_exec_env(config: Dict[str, Any], payload: Dict[str, Any], req: Dict[str, Any]) -> Dict[str, str]:
    env: Dict[str, str] = {}

    for key in SAFE_ENV_KEYS:
        value = os.environ.get(key)
        if value is not None:
            env[key] = value

    custom_env = config.get("env", {})
    if isinstance(custom_env, dict):
        for key, value in custom_env.items():
            key_text = str(key).strip()
            if not key_text:
                continue
            env[key_text] = payload_value_to_env(value)

    env.update(build_payload_env(payload))
    env["DUCTILE_JOB_ID"] = str(req.get("job_id", ""))
    env["DUCTILE_COMMAND"] = str(req.get("command", ""))
    env["DUCTILE_EVENT_TYPE"] = str(req.get("event", {}).get("type", "")) if isinstance(req.get("event"), dict) else ""
    return env


def parse_retry_exit_codes(config: Dict[str, Any]) -> set[int]:
    raw = config.get("retry_on_exit_codes", [])
    if not isinstance(raw, list):
        return set()
    out: set[int] = set()
    for value in raw:
        try:
            out.add(int(value))
        except (TypeError, ValueError):
            continue
    return out


def upstream_label(req: Dict[str, Any]) -> str:
    ctx = req.get("context", {})
    if not isinstance(ctx, dict):
        return "unknown"

    upstream_pipeline = str(ctx.get("ductile_upstream_pipeline", "")).strip()
    upstream_plugin = str(ctx.get("ductile_upstream_plugin", "")).strip()
    if upstream_pipeline and upstream_plugin:
        return f"{upstream_pipeline}:{upstream_plugin}"
    if upstream_plugin:
        return upstream_plugin
    if upstream_pipeline:
        return upstream_pipeline

    pipeline = str(ctx.get("ductile_pipeline", "")).strip()
    plugin = str(ctx.get("ductile_plugin", "")).strip()
    if pipeline and plugin:
        return f"{pipeline}:{plugin}"
    if plugin:
        return plugin
    if pipeline:
        return pipeline
    return "unknown"


def handle_exec(req: Dict[str, Any]) -> Dict[str, Any]:
    config = req.get("config", {})
    if not isinstance(config, dict):
        return plugin_error("request.config must be an object", retry=False)

    command_raw = config.get("command")
    if not command_raw:
        return plugin_error("config.command is required", retry=False)

    if isinstance(command_raw, list):
        command_args = [str(arg) for arg in command_raw]
    else:
        command_args = shlex.split(str(command_raw))

    if not command_args:
        return plugin_error("config.command must not be empty", retry=False)

    event = req.get("event", {})
    payload = {}
    if isinstance(event, dict):
        raw_payload = event.get("payload", {})
        if isinstance(raw_payload, dict):
            payload = raw_payload

    workspace_dir = str(req.get("workspace_dir", "")).strip()
    working_dir = resolve_working_dir(config, workspace_dir)
    if working_dir and not os.path.isdir(working_dir):
        return plugin_error(f"working_dir is not a directory: {working_dir}", retry=False)

    timeout_seconds = coerce_float(config.get("timeout_seconds"), 60.0, minimum=0.001)

    stdout_max = coerce_int(config.get("stdout_max_bytes"), DEFAULT_STDOUT_MAX, minimum=64)
    stderr_max = coerce_int(config.get("stderr_max_bytes"), DEFAULT_STDERR_MAX, minimum=64)
    include_output_in_event = coerce_bool(config.get("include_output_in_event"), True)
    emit_event = coerce_bool(config.get("emit_event"), True)
    event_type = str(config.get("event_type", "sys_exec.completed")).strip() or "sys_exec.completed"
    retry_exit_codes = parse_retry_exit_codes(config)

    env = build_exec_env(config, payload, req)
    expanded_args = expand_env_vars(command_args, env)
    start = time.time()
    try:
        completed = subprocess.run(
            expanded_args,
            shell=False,
            cwd=working_dir,
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout_seconds,
        )
    except subprocess.TimeoutExpired as exc:
        duration_ms = int((time.time() - start) * 1000)
        stderr_raw = exc.stderr or ""
        stderr_text, stderr_truncated = truncate_text(stderr_raw, stderr_max)
        message = f"command timed out after {timeout_seconds:.3f}s"
        logs = [
            {"level": "error", "message": message},
            {"level": "debug", "message": f"command: {' '.join(expanded_args)}"},
        ]
        if stderr_text:
            logs.append({"level": "error", "message": f"stderr: {stderr_text}"})
        result = plugin_error(message, retry=False, logs=logs)
        if emit_event:
            payload_out: Dict[str, Any] = {
                "command": " ".join(expanded_args),
                "exit_code": -1,
                "duration_ms": duration_ms,
                "timed_out": True,
                "stderr_truncated": stderr_truncated,
                "executed_at": iso_now(),
            }
            if include_output_in_event and stderr_text:
                payload_out["stderr"] = stderr_text
            result["events"] = [{"type": event_type, "payload": payload_out}]
        return result
    except Exception as exc:
        return plugin_error(f"command execution failed: {exc}", retry=False)

    duration_ms = int((time.time() - start) * 1000)
    stdout_text, stdout_truncated = truncate_text(completed.stdout or "", stdout_max)
    stderr_text, stderr_truncated = truncate_text(completed.stderr or "", stderr_max)
    exit_code = int(completed.returncode)
    success = exit_code == 0
    upstream = upstream_label(req)

    logs = [
        {
            "level": "info" if success else "error",
            "message": f"command exited with code {exit_code} in {duration_ms}ms (upstream {upstream})",
        },
        {"level": "debug", "message": f"command: {' '.join(expanded_args)}"},
    ]
    if stdout_text:
        logs.append({"level": "info", "message": f"stdout: {stdout_text}"})
    if stderr_text:
        logs.append({"level": "warn", "message": f"stderr: {stderr_text}"})
    if stdout_truncated:
        logs.append({"level": "warn", "message": "stdout truncated"})
    if stderr_truncated:
        logs.append({"level": "warn", "message": "stderr truncated"})

    event_payload: Dict[str, Any] = {
        "command": " ".join(expanded_args),
        "exit_code": exit_code,
        "duration_ms": duration_ms,
        "stdout_truncated": stdout_truncated,
        "stderr_truncated": stderr_truncated,
        "working_dir": working_dir or "",
        "executed_at": iso_now(),
    }
    if include_output_in_event:
        event_payload["stdout"] = stdout_text
        event_payload["stderr"] = stderr_text

    events = [{"type": event_type, "payload": event_payload}] if emit_event else []

    if success:
        return plugin_ok(
            result=f"command exited with code {exit_code} in {duration_ms}ms",
            events=events,
            logs=logs,
        )

    retry = exit_code in retry_exit_codes
    error_message = stderr_text.strip() or f"command failed with exit code {exit_code}"
    response = plugin_error(error_message, retry=retry, logs=logs)
    if events:
        response["events"] = events
    return response


def main() -> int:
    try:
        req = json.load(sys.stdin)
    except Exception as exc:
        json.dump(plugin_error(f"invalid request json: {exc}", retry=False), sys.stdout)
        sys.stdout.write("\n")
        return 0

    if not isinstance(req, dict):
        json.dump(plugin_error("request must be a JSON object", retry=False), sys.stdout)
        sys.stdout.write("\n")
        return 0

    command = str(req.get("command", "")).strip()
    config = req.get("config", {})
    if not isinstance(config, dict):
        config = {}

    if command == "handle":
        resp = handle_exec(req)
    elif command == "health":
        resp = handle_health(config)
    else:
        resp = plugin_error(f"unsupported command: {command}", retry=False)

    json.dump(resp, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
