#!/usr/bin/env python3
"""File handler plugin for Senechal Gateway.

Reads and writes local files with path security validation.
Protocol v1: reads JSON from stdin, writes JSON to stdout.

This plugin demonstrates:
- Secure file operations with path validation
- Event-driven architecture (handle command)
- Field mapping for pipeline compatibility
- Proper error handling and logging
"""
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple


def handle_command(config: Dict[str, Any], state: Dict[str, Any], event: Dict[str, Any]) -> Dict[str, Any]:
    """Handle command - dispatches based on event type or action field.

    Args:
        config: Plugin configuration from config.yaml
        state: Current plugin state
        event: Event envelope containing payload

    Returns:
        Response envelope with status, events, state_updates, and logs
    """
    payload = event.get("payload", {})
    action = payload.get("action", "")
    event_type = event.get("type", "")

    # Dispatch based on action or event type
    if action == "read" or event_type == "file.read_request":
        return read_file(config, state, payload)
    elif action == "write" or event_type.startswith("fabric.completed"):
        return write_file(config, state, payload)
    else:
        return error_response(
            f"Unknown action '{action}'. Specify 'read' or 'write' in payload.action",
            retry=False
        )


def read_file(config: Dict[str, Any], state: Dict[str, Any], payload: Dict[str, Any]) -> Dict[str, Any]:
    """Read a file and emit file.read event.

    Security: All paths validated against allowed_read_paths with realpath resolution.

    Args:
        config: Plugin config with allowed_read_paths
        state: Current state for counters
        payload: Event payload containing file_path

    Returns:
        Success response with file.read event or error response
    """
    file_path = payload.get("file_path")
    if not file_path:
        return error_response("Missing required field 'file_path' in payload", retry=False)

    # Parse allowed paths
    allowed_paths = parse_path_list(config.get("allowed_read_paths"))
    if not allowed_paths:
        return error_response(
            "No allowed_read_paths configured. Add to config.yaml: plugins.file_handler.config.allowed_read_paths",
            retry=False
        )

    # Validate path security
    abs_path, error = validate_path_security(file_path, allowed_paths, "read")
    if error:
        return error_response(error, retry=False)

    # Read file with proper error handling
    try:
        content = Path(abs_path).read_text(encoding='utf-8')
    except FileNotFoundError:
        return error_response(f"File not found: {abs_path}", retry=False)
    except PermissionError:
        return error_response(f"Permission denied reading: {abs_path}", retry=False)
    except UnicodeDecodeError as e:
        return error_response(f"File encoding error: {e}. Expected UTF-8.", retry=False)
    except Exception as e:
        return error_response(f"Failed to read file: {type(e).__name__}: {e}", retry=True)

    # Prepare event payload with field aliases for pipeline compatibility
    filename = os.path.basename(abs_path)
    size_bytes = len(content.encode('utf-8'))

    event_payload = {
        "file_path": abs_path,
        "filename": filename,
        "content": content,
        "text": content,  # Alias for fabric compatibility
        "size_bytes": size_bytes,
    }

    # Propagate pipeline context fields (pattern, output_dir, etc.)
    for field in ["pattern", "output_dir", "output_path"]:
        if field in payload:
            event_payload[field] = payload[field]

    # Update state counters
    reads_count = state.get("reads_count", 0) + 1

    return {
        "status": "ok",
        "events": [
            {
                "type": "file.read",
                "payload": event_payload,
            }
        ],
        "state_updates": {
            "last_read": datetime.now(timezone.utc).isoformat(),
            "reads_count": reads_count,
            "last_file_path": abs_path,
        },
        "logs": [
            {"level": "info", "message": f"Read {size_bytes:,} bytes from {filename}"},
        ],
    }


def write_file(config: Dict[str, Any], state: Dict[str, Any], payload: Dict[str, Any]) -> Dict[str, Any]:
    """Write content to a file and emit file.written event.

    Security: All paths validated against allowed_write_paths with realpath resolution.
    Creates parent directories as needed.

    Args:
        config: Plugin config with allowed_write_paths
        state: Current state for counters
        payload: Event payload containing content/result and output_path/output_dir

    Returns:
        Success response with file.written event or error response
    """
    # Prefer 'result' (processed data from fabric) over 'content' (raw input)
    content = payload.get("result") or payload.get("content")
    if not content:
        return error_response(
            "Missing required field 'content' or 'result' in payload",
            retry=False
        )

    if not isinstance(content, str):
        return error_response(
            f"Content must be string, got {type(content).__name__}",
            retry=False
        )

    # Determine output path
    output_path = payload.get("output_path")
    output_dir = payload.get("output_dir")

    if not output_path and not output_dir:
        return error_response(
            "Missing required field 'output_path' or 'output_dir' in payload",
            retry=False
        )

    # Generate filename if only directory provided
    if not output_path:
        assert output_dir is not None  # Type narrowing for mypy
        timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")
        pattern = payload.get("pattern", "report")
        # Sanitize pattern for filename safety
        safe_pattern = "".join(c if c.isalnum() or c in "-_" else "_" for c in pattern)
        filename = f"{safe_pattern}_{timestamp}.md"
        output_path = os.path.join(output_dir, filename)

    # Parse allowed paths
    allowed_paths = parse_path_list(config.get("allowed_write_paths"))
    if not allowed_paths:
        return error_response(
            "No allowed_write_paths configured. Add to config.yaml: plugins.file_handler.config.allowed_write_paths",
            retry=False
        )

    # Validate path security
    abs_path, error = validate_path_security(output_path, allowed_paths, "write")
    if error:
        return error_response(error, retry=False)

    # Create parent directories
    try:
        Path(abs_path).parent.mkdir(parents=True, exist_ok=True)
    except PermissionError:
        return error_response(f"Permission denied creating directory: {abs_path}", retry=False)
    except Exception as e:
        return error_response(f"Failed to create directory: {type(e).__name__}: {e}", retry=False)

    # Write file
    try:
        Path(abs_path).write_text(content, encoding='utf-8')
    except PermissionError:
        return error_response(f"Permission denied writing: {abs_path}", retry=False)
    except OSError as e:
        return error_response(f"Failed to write file: {type(e).__name__}: {e}", retry=True)
    except Exception as e:
        return error_response(f"Unexpected error writing file: {type(e).__name__}: {e}", retry=True)

    size_bytes = len(content.encode('utf-8'))
    writes_count = state.get("writes_count", 0) + 1

    return {
        "status": "ok",
        "events": [
            {
                "type": "file.written",
                "payload": {
                    "file_path": abs_path,
                    "size_bytes": size_bytes,
                },
            }
        ],
        "state_updates": {
            "last_write": datetime.now(timezone.utc).isoformat(),
            "writes_count": writes_count,
            "last_output_path": abs_path,
        },
        "logs": [
            {"level": "info", "message": f"Wrote {size_bytes:,} bytes to {os.path.basename(abs_path)}"},
        ],
    }


def health_command(config: Dict[str, Any]) -> Dict[str, Any]:
    """Validate configured paths exist and are accessible.

    Args:
        config: Plugin configuration

    Returns:
        Health check response
    """
    issues = []

    # Check read paths
    read_paths = parse_path_list(config.get("allowed_read_paths"))
    for path in read_paths:
        path_obj = Path(path)
        if not path_obj.exists():
            issues.append(f"Read path does not exist: {path}")
        elif not os.access(path, os.R_OK):
            issues.append(f"Read path not readable: {path}")

    # Check write paths
    write_paths = parse_path_list(config.get("allowed_write_paths"))
    for path in write_paths:
        path_obj = Path(path)
        if not path_obj.exists():
            # Try to create it
            try:
                path_obj.mkdir(parents=True, exist_ok=True)
            except Exception as e:
                issues.append(f"Write path does not exist and cannot be created: {path} ({e})")
        elif not os.access(path, os.W_OK):
            issues.append(f"Write path not writable: {path}")

    if issues:
        return error_response(
            f"Health check failed: {'; '.join(issues)}",
            retry=False
        )

    return {
        "status": "ok",
        "state_updates": {
            "last_health_check": datetime.now(timezone.utc).isoformat(),
            "read_paths_configured": len(read_paths),
            "write_paths_configured": len(write_paths),
        },
        "logs": [
            {
                "level": "info",
                "message": f"Health OK: {len(read_paths)} read paths, {len(write_paths)} write paths"
            },
        ],
    }


def parse_path_list(paths: Any) -> List[str]:
    """Parse path configuration (comma-separated string or list).

    Args:
        paths: String with comma-separated paths or list of paths

    Returns:
        List of trimmed, non-empty path strings
    """
    if not paths:
        return []

    if isinstance(paths, str):
        return [p.strip() for p in paths.split(",") if p.strip()]
    elif isinstance(paths, list):
        return [str(p).strip() for p in paths if p]
    else:
        return []


def validate_path_security(
    path: str,
    allowed_prefixes: List[str],
    operation: str
) -> Tuple[Optional[str], Optional[str]]:
    """Validate path is under an allowed prefix using realpath resolution.

    Security: Prevents directory traversal and symlink escape attacks by
    resolving all paths to their canonical absolute form before comparison.

    Args:
        path: Path to validate
        allowed_prefixes: List of allowed directory prefixes
        operation: Operation name for error messages ("read" or "write")

    Returns:
        Tuple of (absolute_path, error_message). If validation succeeds,
        error_message is None. If validation fails, absolute_path is None.
    """
    try:
        abs_path = os.path.realpath(path)
    except Exception as e:
        return None, f"Invalid {operation} path: {type(e).__name__}: {e}"

    # Check if path is under any allowed prefix
    for allowed_prefix in allowed_prefixes:
        try:
            allowed_abs = os.path.realpath(allowed_prefix)
        except Exception:
            continue  # Skip invalid prefixes

        # Path must start with allowed prefix + separator (or be exact match)
        if abs_path == allowed_abs or abs_path.startswith(allowed_abs + os.sep):
            return abs_path, None

    # No matching prefix found
    return None, (
        f"Path not under allowed {operation} prefixes.\n"
        f"  Requested: {abs_path}\n"
        f"  Allowed prefixes: {', '.join(allowed_prefixes)}"
    )


def error_response(message: str, retry: bool = False) -> Dict[str, Any]:
    """Create error response envelope.

    Args:
        message: Error message
        retry: Whether the operation should be retried

    Returns:
        Error response envelope
    """
    return {
        "status": "error",
        "error": message,
        "retry": retry,
        "logs": [{"level": "error", "message": message}],
    }


def main() -> None:
    """Main entry point - read request from stdin, execute command, write response to stdout."""
    try:
        request = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        # Fatal protocol error - write error to stdout and exit
        json.dump(error_response(f"Invalid JSON input: {e}", retry=False), sys.stdout)
        sys.stdout.write("\n")
        sys.exit(1)

    command = request.get("command", "")
    config = request.get("config", {})
    state = request.get("state", {})
    event = request.get("event", {})

    # Dispatch to command handlers
    if command == "poll":
        # Poll command - no-op for event-driven plugin
        response = {
            "status": "ok",
            "state_updates": {"last_poll": datetime.now(timezone.utc).isoformat()},
            "logs": [{"level": "info", "message": "file_handler poll (no-op, event-driven)"}],
        }
    elif command == "handle":
        response = handle_command(config, state, event)
    elif command == "health":
        response = health_command(config)
    else:
        response = error_response(
            f"Unknown command: '{command}'. Supported: poll, handle, health",
            retry=False
        )

    # Write response
    json.dump(response, sys.stdout, indent=None, separators=(',', ':'))
    sys.stdout.write("\n")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
