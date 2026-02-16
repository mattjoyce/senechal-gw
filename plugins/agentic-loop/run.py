#!/usr/bin/env python3
"""Resumable agent loop plugin.

This plugin implements a minimal resumable loop contract:
- Start event (`agentic.start`) creates a run and emits one tool request.
- Tool result event (`agentic.tool_result`) validates correlation and either:
  - emits a follow-up tool request, or
  - emits `agent.completed`, or
  - emits `agent.escalated` on protocol/policy errors.

It intentionally avoids nested synchronous API calls and special process exits.
"""

import json
import re
import sys
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional, Tuple
from uuid import uuid4


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def log(level: str, message: str) -> Dict[str, str]:
    return {"level": level, "message": message}


def ok_response(
    *,
    events: Optional[List[Dict[str, Any]]] = None,
    state_updates: Optional[Dict[str, Any]] = None,
    logs: Optional[List[Dict[str, str]]] = None,
) -> Dict[str, Any]:
    return {
        "status": "ok",
        "events": events or [],
        "state_updates": state_updates or {},
        "logs": logs or [],
    }


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [log("error", message)],
    }


def extract_first_url(text: str) -> Optional[str]:
    match = re.search(r"https?://[^\s)]+", text or "")
    return match.group(0) if match else None


def normalize_runs(state: Dict[str, Any]) -> Dict[str, Any]:
    runs = state.get("runs", {})
    if isinstance(runs, dict):
        return runs
    return {}


def coerce_int(value: Any, default: int) -> int:
    try:
        result = int(value)
    except (TypeError, ValueError):
        return default
    return result if result > 0 else default


def config_list(config: Dict[str, Any], key: str) -> List[str]:
    value = config.get(key, [])
    if not isinstance(value, list):
        return []
    out: List[str] = []
    for item in value:
        if isinstance(item, str) and item.strip():
            out.append(item.strip())
    return out


def choose_first_tool(
    goal: str, context: Dict[str, Any], config: Dict[str, Any]
) -> Tuple[str, str, Dict[str, Any]]:
    # Highest precedence: explicit event context.
    tool = context.get("tool") or context.get("initial_tool")
    tool_command = context.get("tool_command") or config.get("default_tool_command") or "handle"
    provided_payload = context.get("tool_payload")
    tool_payload = provided_payload if isinstance(provided_payload, dict) else {}

    if not isinstance(tool, str) or not tool.strip():
        tool = config.get("default_tool")

    if not isinstance(tool, str) or not tool.strip():
        url = extract_first_url(goal)
        tool = "jina-reader" if url else "fabric"
        if url and "url" not in tool_payload:
            tool_payload["url"] = url

    if tool == "jina-reader" and "url" not in tool_payload:
        url = extract_first_url(goal)
        if url:
            tool_payload["url"] = url

    return tool, str(tool_command), tool_payload


def make_tool_request_event(
    run_id: str,
    step: int,
    tool: str,
    tool_command: str,
    tool_payload: Dict[str, Any],
) -> Dict[str, Any]:
    return {
        "type": "agentic.tool_request",
        "payload": {
            "run_id": run_id,
            "step": step,
            "tool": tool,
            "tool_command": tool_command,
            "tool_payload": tool_payload,
            "requested_at": now_iso(),
        },
        "dedupe_key": f"agentic:run:{run_id}:step:{step}:request",
    }


def run_start(
    config: Dict[str, Any], state: Dict[str, Any], event: Dict[str, Any]
) -> Dict[str, Any]:
    payload = event.get("payload", {})
    if not isinstance(payload, dict):
        return error_response("agentic.start payload must be an object", retry=False)

    goal = payload.get("goal")
    if not isinstance(goal, str) or not goal.strip():
        return error_response("agentic.start requires non-empty payload.goal", retry=False)

    context = payload.get("context")
    if not isinstance(context, dict):
        context = {}

    runs = normalize_runs(state)
    run_id = payload.get("run_id")
    if not isinstance(run_id, str) or not run_id.strip():
        run_id = str(uuid4())

    max_steps_cfg = coerce_int(config.get("max_steps"), 20)
    max_steps = coerce_int(context.get("max_steps"), max_steps_cfg)
    max_reframes = coerce_int(config.get("max_reframes"), 2)

    tool, tool_command, tool_payload = choose_first_tool(goal, context, config)
    allowed_plugins = config_list(config, "allowed_plugins")
    if allowed_plugins and tool not in allowed_plugins:
        return error_response(
            f"tool '{tool}' is not allowed; allowed_plugins={allowed_plugins}",
            retry=False,
        )

    run = {
        "status": "running",
        "goal": goal,
        "created_at": now_iso(),
        "updated_at": now_iso(),
        "step": 1,
        "max_steps": max_steps,
        "reframes": 0,
        "max_reframes": max_reframes,
        "pending_step": 1,
        "pending_tool": tool,
        "pending_since": now_iso(),
        "last_tool_command": tool_command,
    }
    runs[run_id] = run

    tool_request = make_tool_request_event(
        run_id=run_id,
        step=1,
        tool=tool,
        tool_command=tool_command,
        tool_payload=tool_payload,
    )

    return ok_response(
        events=[tool_request],
        state_updates={"runs": runs, "last_run_id": run_id},
        logs=[
            log("info", f"started run {run_id}"),
            log("info", f"step=1 pending_tool={tool}"),
        ],
    )


def run_tool_result(
    config: Dict[str, Any], state: Dict[str, Any], event: Dict[str, Any]
) -> Dict[str, Any]:
    payload = event.get("payload", {})
    if not isinstance(payload, dict):
        return error_response("agentic.tool_result payload must be an object", retry=False)

    run_id = payload.get("run_id")
    tool = payload.get("tool")
    step = payload.get("step")
    status = payload.get("status")
    result = payload.get("result")
    error = payload.get("error")

    if not isinstance(run_id, str) or not run_id:
        return error_response("agentic.tool_result missing payload.run_id", retry=False)
    if not isinstance(tool, str) or not tool:
        return error_response("agentic.tool_result missing payload.tool", retry=False)
    step_num = coerce_int(step, -1)
    if step_num < 1:
        return error_response("agentic.tool_result missing or invalid payload.step", retry=False)
    if not isinstance(status, str) or not status:
        return error_response("agentic.tool_result missing payload.status", retry=False)

    runs = normalize_runs(state)
    run = runs.get(run_id)
    if not isinstance(run, dict):
        return ok_response(logs=[log("warn", f"unknown run_id={run_id}; ignoring result event")])

    if run.get("status") != "running":
        return ok_response(
            logs=[log("info", f"run_id={run_id} already terminal ({run.get('status')}); ignoring")]
        )

    pending_step = coerce_int(run.get("pending_step"), -1)
    pending_tool = run.get("pending_tool")

    if step_num < pending_step:
        return ok_response(
            logs=[log("info", f"stale result for run_id={run_id}, step={step_num}; pending={pending_step}")]
        )

    if step_num != pending_step:
        return error_response(
            f"protocol violation: step mismatch run_id={run_id} pending_step={pending_step} got={step_num}",
            retry=False,
        )

    if tool != pending_tool:
        run["status"] = "escalated"
        run["updated_at"] = now_iso()
        runs[run_id] = run
        return ok_response(
            events=[
                {
                    "type": "agent.escalated",
                    "payload": {
                        "run_id": run_id,
                        "reason": "pending_tool_mismatch",
                        "expected_tool": pending_tool,
                        "actual_tool": tool,
                    },
                }
            ],
            state_updates={"runs": runs},
            logs=[log("error", f"run {run_id} escalated: expected_tool={pending_tool} actual_tool={tool}")],
        )

    if status != "ok":
        run["status"] = "escalated"
        run["updated_at"] = now_iso()
        runs[run_id] = run
        return ok_response(
            events=[
                {
                    "type": "agent.escalated",
                    "payload": {
                        "run_id": run_id,
                        "reason": "tool_error",
                        "step": step_num,
                        "tool": tool,
                        "error": error or "unknown tool error",
                    },
                }
            ],
            state_updates={"runs": runs},
            logs=[log("error", f"run {run_id} escalated from tool error at step={step_num}")],
        )

    max_steps = coerce_int(run.get("max_steps"), 20)
    if step_num >= max_steps:
        run["status"] = "escalated"
        run["updated_at"] = now_iso()
        runs[run_id] = run
        return ok_response(
            events=[
                {
                    "type": "agent.escalated",
                    "payload": {
                        "run_id": run_id,
                        "reason": "max_steps_exceeded",
                        "step": step_num,
                        "max_steps": max_steps,
                    },
                }
            ],
            state_updates={"runs": runs},
            logs=[log("error", f"run {run_id} escalated: max_steps exceeded")],
        )

    # Minimal deterministic two-step flow:
    # 1) expected first tool is content fetch (often jina-reader)
    # 2) second tool is fabric to generate final critique
    if tool == "jina-reader":
        next_step = step_num + 1
        critique_prompt = (
            "Write a constructive two-paragraph critique of the supplied webpage content."
        )
        source_text = ""
        if isinstance(result, dict):
            source_text = str(result.get("text") or result.get("excerpt") or "")
        elif isinstance(result, str):
            source_text = result

        next_payload = {"prompt": critique_prompt}
        if source_text:
            next_payload["text"] = source_text

        next_tool = "fabric"
        allowed_plugins = config_list(config, "allowed_plugins")
        if allowed_plugins and next_tool not in allowed_plugins:
            run["status"] = "escalated"
            run["updated_at"] = now_iso()
            runs[run_id] = run
            return ok_response(
                events=[
                    {
                        "type": "agent.escalated",
                        "payload": {
                            "run_id": run_id,
                            "reason": "followup_tool_not_allowed",
                            "tool": next_tool,
                        },
                    }
                ],
                state_updates={"runs": runs},
                logs=[log("error", f"run {run_id} escalated: follow-up tool '{next_tool}' not allowed")],
            )

        run["step"] = next_step
        run["pending_step"] = next_step
        run["pending_tool"] = next_tool
        run["pending_since"] = now_iso()
        run["updated_at"] = now_iso()
        run["last_tool_command"] = "handle"
        runs[run_id] = run

        return ok_response(
            events=[
                make_tool_request_event(
                    run_id=run_id,
                    step=next_step,
                    tool=next_tool,
                    tool_command="handle",
                    tool_payload=next_payload,
                )
            ],
            state_updates={"runs": runs},
            logs=[log("info", f"run {run_id} advanced to step={next_step} tool={next_tool}")],
        )

    # Default terminal path: mark run done after any non-fetch tool result.
    run["status"] = "done"
    run["updated_at"] = now_iso()
    run["pending_step"] = None
    run["pending_tool"] = None
    runs[run_id] = run

    outcome = ""
    artifacts: List[str] = []
    if isinstance(result, dict):
        maybe_outcome = result.get("result") or result.get("summary") or ""
        if isinstance(maybe_outcome, str):
            outcome = maybe_outcome
        artifact_path = result.get("artifact_path") or result.get("output_path")
        if isinstance(artifact_path, str) and artifact_path:
            artifacts.append(artifact_path)
    elif isinstance(result, str):
        outcome = result

    if not outcome:
        outcome = f"Run {run_id} completed at step {step_num}"

    return ok_response(
        events=[
            {
                "type": "agent.completed",
                "payload": {
                    "run_id": run_id,
                    "goal": run.get("goal", ""),
                    "outcome": outcome,
                    "steps_taken": step_num,
                    "artifacts": artifacts,
                },
            }
        ],
        state_updates={"runs": runs, "last_run_id": run_id},
        logs=[log("info", f"run {run_id} completed in {step_num} step(s)")],
    )


def poll_command(state: Dict[str, Any]) -> Dict[str, Any]:
    runs = normalize_runs(state)
    running = sum(1 for _, v in runs.items() if isinstance(v, dict) and v.get("status") == "running")
    return ok_response(
        state_updates={"last_poll": now_iso()},
        logs=[log("info", f"agentic-loop poll noop; running_runs={running}")],
    )


def health_command(config: Dict[str, Any], state: Dict[str, Any]) -> Dict[str, Any]:
    max_steps = coerce_int(config.get("max_steps"), 20)
    runs = normalize_runs(state)
    return ok_response(
        state_updates={"last_health_check": now_iso(), "configured_max_steps": max_steps},
        logs=[log("info", f"healthy; tracked_runs={len(runs)}")],
    )


def handle_command(config: Dict[str, Any], state: Dict[str, Any], event: Dict[str, Any]) -> Dict[str, Any]:
    event_type = event.get("type", "")
    if event_type in ("agentic.start", "api.trigger"):
        return run_start(config, state, event)
    if event_type == "agentic.tool_result":
        return run_tool_result(config, state, event)
    return error_response(
        f"unsupported event type '{event_type}'; expected agentic.start, api.trigger, or agentic.tool_result",
        retry=False,
    )


def main() -> None:
    try:
        request = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        json.dump(error_response(f"invalid request JSON: {exc}", retry=False), sys.stdout)
        return

    command = request.get("command", "")
    config = request.get("config", {})
    state = request.get("state", {})
    event = request.get("event", {})

    if not isinstance(config, dict):
        config = {}
    if not isinstance(state, dict):
        state = {}
    if not isinstance(event, dict):
        event = {}

    if command == "poll":
        response = poll_command(state)
    elif command == "handle":
        response = handle_command(config, state, event)
    elif command == "health":
        response = health_command(config, state)
    else:
        response = error_response(f"unknown command: {command}", retry=False)

    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
