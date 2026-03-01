# YAML Tips for Ductile Configuration

Practical techniques for keeping your `plugins.yaml` clean and maintainable.
No prior YAML expertise assumed.

---

## Anchors and Aliases: Stop Repeating Yourself

### The problem

When you create multiple plugin instances that share the same base settings,
you end up copy-pasting the same values over and over:

```yaml
plugins:
  discord_test_cron:
    uses: discord_notify
    enabled: true
    timeout: 15s
    max_attempts: 2
    schedules:
      - cron: "*/5 * * * *"
        command: poll
    config:
      webhook_url: "https://discord.com/api/webhooks/123/abc..."
      default_username: "Ductile"
      poll_message: "[T2] cron */5min"

  discord_test_window:
    uses: discord_notify
    enabled: true       # repeated
    timeout: 15s        # repeated
    max_attempts: 2     # repeated
    schedules:
      - every: 3m
        only_between: "07:00-22:00"
        command: poll
    config:
      webhook_url: "https://discord.com/api/webhooks/123/abc..."  # repeated
      default_username: "Ductile"                                  # repeated
      poll_message: "[T3] only_between 07-22"
```

If you want to change the timeout or webhook URL, you have to edit every block.
That's fragile and error-prone.

### The solution: YAML anchors

YAML has a built-in mechanism for this called **anchors** (`&`) and **aliases** (`*`).
You define a block once with an anchor, then reference it by name anywhere else.
The YAML parser expands the reference before any application ever reads the file —
it's a pure YAML feature, not a ductile feature.

#### Step 1 — Define an anchor

An anchor is a name you attach to any YAML value using `&name`. It can go on a
mapping (dict), a sequence (list), or a scalar (string/number).

```yaml
# This defines an anchor called "discord-test" on this mapping block.
# The key name ("x-discord-test") is arbitrary — pick something descriptive.
x-discord-test: &discord-test
  uses: discord_notify
  enabled: true
  timeout: 15s
  max_attempts: 2
```

#### Step 2 — Reference it with an alias

Wherever you want to reuse that block, write `*anchor-name`. The YAML parser
replaces it with the full content of the anchored block.

```yaml
discord_test_cron:
  *discord-test          # expands to: uses, enabled, timeout, max_attempts
  schedules:
    - cron: "*/5 * * * *"
      command: poll
```

But wait — you also need to *add* fields on top of the expanded block (like
`schedules`). For that, use the **merge key** `<<:`.

#### Step 3 — Merge with `<<:`

`<<: *anchor-name` merges the referenced block's keys into the current mapping.
Keys you define explicitly take priority over merged ones.

```yaml
discord_test_cron:
  <<: *discord-test     # merges: uses, enabled, timeout, max_attempts
  schedules:            # adds this new key on top
    - cron: "*/5 * * * *"
      command: poll
```

This is equivalent to writing all four merged keys out explicitly, plus the
`schedules` key.

#### Step 4 — Anchors work on nested mappings too

You can anchor the `config:` sub-block separately:

```yaml
x-discord-config: &discord-config
  webhook_url: "https://discord.com/api/webhooks/123/abc..."
  default_username: "Ductile"
```

Then merge it inside the `config:` block of each plugin, adding only the
field that differs:

```yaml
discord_test_cron:
  <<: *discord-test
  schedules:
    - cron: "*/5 * * * *"
      command: poll
  config:
    <<: *discord-config        # merges: webhook_url, default_username
    poll_message: "[T2] cron"  # adds the unique field
```

### Why this works with ductile

There are two things worth understanding:

**1. The YAML parser resolves anchors before ductile sees anything.**

When ductile loads `plugins.yaml`, the YAML library reads the file and fully
expands all anchors and merge keys first. By the time ductile's config loader
processes the result, it just sees a normal, fully-populated config map. Ductile
has no idea anchors were used.

**2. Unknown top-level keys are silently ignored.**

The anchor definitions live at the *top level* of the YAML file, outside the
`plugins:` block. Ductile's config loader only reads keys it knows about
(`plugins:`, `service:`, `pipelines:`, etc.). Anything else — including
`x-discord-test:` and `x-discord-config:` — is silently ignored.

This is why the anchor names are prefixed with `x-` by convention: it signals
"this is application-level metadata, not ductile config". Any name works, but
a consistent prefix avoids confusion.

### Full before/after example

**Before** (80 lines, webhook URL repeated 5 times):

```yaml
plugins:
  discord_test_cron:
    uses: discord_notify
    enabled: true
    timeout: 15s
    max_attempts: 2
    schedules:
      - cron: "*/5 * * * *"
        command: poll
    config:
      webhook_url: "https://discord.com/api/webhooks/123/abc..."
      default_username: "Ductile Scheduler"
      poll_message: "[T2] cron */5min"

  discord_test_window:
    uses: discord_notify
    enabled: true
    timeout: 15s
    max_attempts: 2
    schedules:
      - every: 3m
        only_between: "07:00-22:00"
        command: poll
    config:
      webhook_url: "https://discord.com/api/webhooks/123/abc..."
      default_username: "Ductile Scheduler"
      poll_message: "[T3] only_between 07-22"

  # ... and so on for each test instance
```

**After** (webhook URL in one place, instances are concise):

```yaml
# Shared base — expanded by YAML parser, ignored as a key by ductile
x-discord-test: &discord-test
  uses: discord_notify
  enabled: true
  timeout: 15s
  max_attempts: 2

x-discord-config: &discord-config
  webhook_url: "https://discord.com/api/webhooks/123/abc..."
  default_username: "Ductile Scheduler"

plugins:
  discord_test_cron:
    <<: *discord-test
    schedules:
      - cron: "*/5 * * * *"
        command: poll
    config:
      <<: *discord-config
      poll_message: "[T2] cron */5min"

  discord_test_window:
    <<: *discord-test
    schedules:
      - every: 3m
        only_between: "07:00-22:00"
        command: poll
    config:
      <<: *discord-config
      poll_message: "[T3] only_between 07-22"
```

### Caveats

**Merge is shallow.** `<<:` merges the *top-level keys* of the anchored block.
It does not deep-merge nested structures. If your anchor contains a `config:`
block, and you also define a `config:` block in the instance, the instance's
`config:` block wins entirely — the anchor's `config:` is not merged into it.
This is why the example anchors `config` separately (as `*discord-config`) and
merges it *inside* the `config:` block.

**Explicit keys win over merged keys.** If a key appears in both the `<<:`
block and the current mapping, the explicitly written key takes precedence.
Use this to override specific values from the shared base.

**Anchors are file-scoped.** An anchor defined in `plugins.yaml` cannot be
referenced from `pipelines.yaml`. If you use modular config files, you need
to repeat shared values across files or consolidate into a single file.

**`config check` still validates the expanded result.** Anchors are transparent
to ductile's validator. If a merged value is invalid, the error will point to
the plugin instance, not the anchor definition.

---

## Further reading

- [YAML specification — anchors and aliases](https://yaml.org/spec/1.2.2/#anchors-and-aliases)
- [YAML specification — merge keys](https://yaml.org/type/merge.html)
- Ductile config reference: `CONFIG_REFERENCE.md`
- Plugin instance aliasing (`uses:`): `COOKBOOK.md`
