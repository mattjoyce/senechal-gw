# file_handler

Read and write local files with path allow-lists. Useful for connecting pipelines to the filesystem.

## Commands
- `poll` (write): No-op scheduled tick.
- `handle` (write): Read or write a file.
- `health` (read): Validate configured paths.

## Configuration
- `allowed_read_paths`: List or comma-separated string of readable root paths.
- `allowed_write_paths`: List or comma-separated string of writable root paths.
- `default_output_dir`: Default directory for writes when `output_path` is not provided.

## Input (handle)
- For reads: `action: read`, `file_path`.
- For writes: `action: write`, `content` or `result`, and `output_path` or `output_dir`.

## Events
- `file.read` with payload containing `file_path`, `content`, `text`, `size_bytes`.
- `file.written` with payload containing `file_path`, `size_bytes`.

## Example
```yaml
plugins:
  file_handler:
    enabled: true
    config:
      allowed_read_paths:
        - /var/data
      allowed_write_paths:
        - /var/output
      default_output_dir: /var/output
```
