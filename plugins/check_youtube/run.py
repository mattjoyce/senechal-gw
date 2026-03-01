#!/usr/bin/env python3
"""if classifier plugin for Ductile (protocol v2).

NOTE: This is a copy of the base `if` plugin with a different manifest name
for instance-style configuration. Keep logic in sync with plugins/if/run.py.

Evaluates configured checks against a payload field and emits the first
matching event type with the payload unchanged.
"""

from __future__ import annotations

import json
import re
import sys
from typing import Any, Dict, Iterable, List, Optional, Tuple

SUPPORTED_CONDITIONS = {"contains", "startswith", "endswith", "equals", "regex", "default"}


def error_response(message: str, logs: Optional[List[Dict[str, str]]] = None) -> Dict[str, Any]:
    return {
        "status": "error",
        "error": message,
        "retry": False,
        "logs": logs or [{"level": "error", "message": message}],
    }


def ok_response(result: str, events: Optional[List[Dict[str, Any]]] = None, logs: Optional[List[Dict[str, str]]] = None) -> Dict[str, Any]:
    return {
        "status": "ok",
        "result": result,
        "events": events or [],
        "logs": logs or [],
    }


def resolve_field_value(event: Dict[str, Any], field: str) -> str:
    if not field:
        return ""

    path = str(field).strip()
    if not path:
        return ""

    payload = event.get("payload") if isinstance(event.get("payload"), dict) else {}
    if path == "payload":
        value: Any = payload
        return "" if value is None else str(value)

    if path == "event":
        return json.dumps(event, ensure_ascii=False)

    if path.startswith("payload."):
        value = payload
        path = path[len("payload.") :]
    elif path.startswith("event."):
        value = event
        path = path[len("event.") :]
    else:
        value = payload

    if path:
        for part in path.split("."):
            if isinstance(value, dict) and part in value:
                value = value[part]
            else:
                return ""

    if value is None:
        return ""
    return str(value)


def parse_check(check: Dict[str, Any]) -> Tuple[str, Any, str]:
    cond_types = [key for key in SUPPORTED_CONDITIONS if key in check]
    if len(cond_types) != 1:
        raise ValueError("check must include exactly one condition type")

    cond_type = cond_types[0]
    emit = check.get("emit")
    if cond_type == "default" and emit is None:
        emit = check.get("default")

    if emit is None or str(emit).strip() == "":
        raise ValueError(f"check {cond_type} requires emit")

    cond_value = check.get(cond_type)
    if cond_type != "default" and (cond_value is None or str(cond_value).strip() == ""):
        raise ValueError(f"check {cond_type} requires a value")

    return cond_type, cond_value, str(emit)


def match_condition(value: str, cond_type: str, cond_value: Any) -> bool:
    if cond_type == "default":
        return True

    candidate = value or ""
    cond_str = "" if cond_value is None else str(cond_value)

    if cond_type == "regex":
        return re.fullmatch(cond_str, candidate) is not None

    candidate_folded = candidate.casefold()
    cond_folded = cond_str.casefold()

    if cond_type == "contains":
        return cond_folded in candidate_folded
    if cond_type == "startswith":
        return candidate_folded.startswith(cond_folded)
    if cond_type == "endswith":
        return candidate_folded.endswith(cond_folded)
    if cond_type == "equals":
        return candidate_folded == cond_folded

    raise ValueError(f"unsupported condition type: {cond_type}")


def validate_config(config: Dict[str, Any]) -> List[str]:
    errors: List[str] = []
    field = config.get("field")
    checks = config.get("checks")

    if field is None or str(field).strip() == "":
        errors.append("config.field is required")

    if not isinstance(checks, list) or len(checks) == 0:
        errors.append("config.checks must be a non-empty list")
        return errors

    for idx, check in enumerate(checks):
        if not isinstance(check, dict):
            errors.append(f"check #{idx + 1} must be an object")
            continue
        try:
            parse_check(check)
        except ValueError as exc:
            errors.append(f"check #{idx + 1}: {exc}")

    return errors


def handle_command(config: Dict[str, Any], event: Dict[str, Any]) -> Dict[str, Any]:
    errors = validate_config(config)
    if errors:
        return error_response("; ".join(errors))

    field = str(config.get("field", ""))
    checks = config.get("checks", [])
    payload = event.get("payload") if isinstance(event.get("payload"), dict) else {}
    input_dedupe_key = event.get("dedupe_key")

    value = resolve_field_value(event, field)

    for check in checks:
        try:
            cond_type, cond_value, emit = parse_check(check)
        except ValueError as exc:
            return error_response(str(exc))

        try:
            if match_condition(value, cond_type, cond_value):
                output_event = {"type": emit, "payload": payload}
                if input_dedupe_key:
                    output_event["dedupe_key"] = input_dedupe_key
                result = f"matched {cond_type} -> {emit}"
                return ok_response(
                    result,
                    events=[output_event],
                    logs=[{"level": "info", "message": result}],
                )
        except re.error as exc:
            return error_response(f"invalid regex: {exc}")

    return error_response("no checks matched and no default provided")


def health_command(config: Dict[str, Any]) -> Dict[str, Any]:
    errors = validate_config(config)
    if errors:
        return error_response("; ".join(errors))

    return ok_response("config ok", logs=[{"level": "info", "message": "config ok"}])


def handle_request(request: Dict[str, Any]) -> Dict[str, Any]:
    command = request.get("command", "")
    config = request.get("config") if isinstance(request.get("config"), dict) else {}
    event = request.get("event") if isinstance(request.get("event"), dict) else {}

    if command == "handle":
        return handle_command(config, event)
    if command == "health":
        return health_command(config)

    return error_response(f"unknown command: {command}")


def main() -> None:
    request = json.load(sys.stdin)
    response = handle_request(request)
    json.dump(response, sys.stdout)


if __name__ == "__main__":
    main()
