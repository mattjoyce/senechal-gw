# Sprint 3 Live Config Baggage Migration Proposal

Date: 2026-04-19
Branch: `hickey-sprint-3-explicit-durability`
Live config reviewed: `/home/matt/.config/ductile/pipelines.yaml`

This note proposes how to migrate the live Ductile pipeline config toward
Sprint 3 explicit durability. It is intentionally a proposal only. Do not edit
the live config until the author has reviewed the durable names.

## Current State

The live config currently has:

- 22 pipelines
- 36 top-level steps
- 0 `baggage:` blocks
- 1 existing `with:` block

The branch keeps transition compatibility: steps without `baggage:` still use
legacy payload promotion and emit transition warnings. That means the live
config does not have to be fully migrated before this branch can be tested, but
strict mode should be introduced pipeline by pipeline.

Likely strict migration surface:

- high priority: 8-10 multi-step pipelines
- lower priority: single-step terminal/notification pipelines
- review needed: hook/event pipelines whose payload shape is not yet documented

## Author Model

Use these rules when editing config:

- `payload` is the immediate event body for the current step.
- `with:` shapes only the immediate request to the plugin.
- `baggage:` names durable facts for the execution lineage.
- Durable names are immutable deep JSON paths.
- Authors supply namespaces; plugin/default namespace lookup is deferred.
- Do not put large transient bodies in baggage unless there is a clear audit
  reason.

Examples:

```yaml
baggage:
  web.url: payload.url
  summary.text: payload.result
```

```yaml
baggage:
  - from: payload.metadata
    namespace: whisper
```

## Naming Guidance

Prefer domain names over transport names:

- good: `youtube.video_id`, `web.url`, `summary.text`, `repo.path`
- avoid: `result.value`, `message.content`, `payload.url`

Generic plugin output fields should be renamed at the point they become
durable:

- `payload.result` from `fabric` becomes `summary.text`
- `payload.result` from health ETL becomes `healthdata.result`
- `payload.path` from repo sync becomes `repo.path`
- `payload.changed` from repo policy becomes `repo.policy.changed`

## High-Confidence Migration Candidates

### `astro-rebuild-staging-on-summary-change`

Purpose: preserve the folder-watch cause and rebuild outcome, while shaping the
notify message explicitly because `astro_rebuild_notify` templates against
payload, not context.

```yaml
  - name: astro-rebuild-staging-on-summary-change
    on: astro.summaries.changed
    steps:
      - id: rebuild_staging
        uses: astro_rebuild_staging
        baggage:
          astro.summaries.watch_id: payload.watch_id
          astro.summaries.root: payload.root
          astro.summaries.changed_count: payload.changed_count
          astro.summaries.created: payload.created
          astro.summaries.modified: payload.modified
          astro.summaries.deleted: payload.deleted
      - id: notify
        uses: astro_rebuild_notify
        with:
          message: "Staging rebuilt - {context.astro.summaries.changed_count} summary file(s) updated."
        baggage:
          rebuild.exit_code: payload.exit_code
          rebuild.duration_ms: payload.duration_ms
          rebuild.executed_at: payload.executed_at
```

Confirmed: `with:` interpolation supports `context.*` and `payload.*`
(`internal/dispatch/with.go`, covered by `internal/dispatch/with_test.go`).

### `youtube-wisdom`

Purpose: preserve request and video identity, not the full transcript by
default. The next step still gets the immediate payload, so baggage does not
need to carry the transcript for normal operation.

```yaml
  - name: youtube-wisdom
    on: youtube.url.detected
    steps:
      - id: transcript
        uses: youtube_transcript
        baggage:
          youtube.input_url: payload.url
          request.prompt: payload.prompt
          request.output_dir: payload.output_dir
          request.id: payload.request_id
          request.source: payload.request_source
      - id: summarize
        uses: fabric
        baggage:
          youtube.video_id: payload.video_id
          youtube.video_url: payload.video_url
          youtube.title: payload.title
          youtube.language: payload.language
          youtube.source_format: payload.source_format
          summary.pattern: payload.pattern
      - id: write
        uses: file_handler
        baggage:
          summary.text: payload.result
          summary.input_length: payload.input_length
          summary.output_length: payload.output_length
          file.output_dir: payload.output_dir
          file.filename: payload.filename
```

Review point: only claim `request.output_path`, `request.filename`, or
`file.output_path` if the triggering event guarantees those fields. Missing
source paths now reject the claim.

### `playlist-wisdom`

Purpose: same as `youtube-wisdom`, plus playlist identity. This needs one pass
against real playlist events before applying because playlist seed fields may
vary.

```yaml
  - name: playlist-wisdom
    on: youtube.playlist_item
    steps:
      - id: transcript
        uses: youtube_transcript
        baggage:
          playlist.id: payload.playlist_id
          playlist.url: payload.playlist_url
          youtube.input_url: payload.url
          request.prompt: payload.prompt
      - id: summarize
        uses: fabric
        baggage:
          youtube.video_id: payload.video_id
          youtube.video_url: payload.video_url
          youtube.title: payload.title
          youtube.language: payload.language
          youtube.source_format: payload.source_format
          summary.pattern: payload.pattern
      - id: write
        uses: file_handler
        baggage:
          summary.text: payload.result
          summary.input_length: payload.input_length
          summary.output_length: payload.output_length
```

Review point: check whether `playlist_id`, `playlist_url`, and `url` are always
present together.

### `web-summarize`

Purpose: preserve URL and fetch metadata, not the full fetched body unless
audit requires it.

```yaml
  - name: web-summarize
    on: web.url.detected
    steps:
      - id: fetch
        uses: jina-reader
        baggage:
          web.input_url: payload.url
      - id: summarize
        uses: fabric
        baggage:
          web.url: payload.url
          web.content_hash: payload.content_hash
          web.truncated: payload.truncated
      - id: write
        uses: file_handler
        baggage:
          summary.text: payload.result
          summary.input_length: payload.input_length
          summary.output_length: payload.output_length
```

Review point: if fetched content must be durable for audit/replay, add
`web.content: payload.content` deliberately. Do not add it by habit.

### `github-repo-sync`

Purpose: preserve discovered repository identity before the sync step emits
follow-up events.

```yaml
  - name: github-repo-sync
    on: github_repo_sync.repo_discovered
    steps:
      - id: sync
        uses: git_repo_sync
        baggage:
          repo.owner: payload.owner
          repo.owner_type: payload.owner_type
          repo.name: payload.repo_name
          repo.full_name: payload.full_name
          repo.clone_url: payload.clone_url
          repo.ssh_url: payload.ssh_url
          repo.clone_dir: payload.clone_dir
          repo.default_branch: payload.default_branch
          repo.pushed_at: payload.pushed_at
```

### `repo-compliance`

Purpose: preserve git sync facts and policy outcome.

```yaml
  - name: repo-compliance
    on: git_repo_sync.completed
    steps:
      - id: policy
        uses: repo_policy
        baggage:
          repo.owner: payload.owner
          repo.name: payload.repo_name
          repo.path: payload.path
          repo.action: payload.action
          repo.new_commits: payload.new_commits
          repo.commit_count: payload.commit_count
          repo.before_sha: payload.before_sha
          repo.after_sha: payload.after_sha
          repo.default_branch: payload.default_branch
          repo.clone_url: payload.clone_url
          repo.ssh_url: payload.ssh_url
      - id: commit
        uses: git_commit_push
        if:
          path: payload.changed
          op: eq
          value: true
        baggage:
          repo.policy.changed: payload.changed
          repo.policy.files_changed: payload.files_changed
```

Review point: the `commit` condition remains immediate-payload behavior. It
does not need to become `context.repo.policy.changed`.

### `repo-changelog`

Purpose: same root git sync facts as `repo-compliance`, with changelog output
becoming durable only after confirming the plugin's output names.

```yaml
  - name: repo-changelog
    on: git_repo_sync.completed
    steps:
      - id: changelog
        uses: changelog_microblog
        if:
          path: payload.new_commits
          op: eq
          value: true
        baggage:
          repo.owner: payload.owner
          repo.name: payload.repo_name
          repo.path: payload.path
          repo.action: payload.action
          repo.new_commits: payload.new_commits
          repo.commit_count: payload.commit_count
          repo.before_sha: payload.before_sha
          repo.after_sha: payload.after_sha
          repo.default_branch: payload.default_branch
          repo.clone_url: payload.clone_url
          repo.ssh_url: payload.ssh_url
      - id: commit
        uses: git_commit_push
        if:
          path: payload.changed
          op: eq
          value: true
        baggage:
          repo.changelog.changed: payload.changed
          repo.changelog.files_changed: payload.files_changed
```

Review point: confirm whether `changelog_microblog` always emits `changed` and
`files_changed`. If it emits a post body or summary, decide whether that should
be durable as `repo.changelog.text`.

### `garmin-daily-summary`

Purpose: preserve the SQLite-change cause and the health ETL outcome. Shape the
notification from immediate ETL output.

```yaml
  - name: garmin-daily-summary
    on: data.change.garmin
    steps:
      - id: etl
        uses: run_healthdata_etl
        baggage:
          garmin.db_path: payload.db_path
          garmin.query: payload.query
          garmin.result: payload.result
          garmin.previous_result: payload.previous_result
          garmin.detected_at: payload.detected_at
      - id: notify
        uses: discord_notify
        with:
          message: "{payload.result}"
        baggage:
          healthdata.source: payload.source
          healthdata.result: payload.result
```

### `withings-weight-summary`

Purpose: same as Garmin, namespaced for Withings.

```yaml
  - name: withings-weight-summary
    on: data.change.withings
    steps:
      - id: etl
        uses: run_healthdata_etl
        baggage:
          withings.db_path: payload.db_path
          withings.query: payload.query
          withings.result: payload.result
          withings.previous_result: payload.previous_result
          withings.detected_at: payload.detected_at
      - id: notify
        uses: discord_notify
        with:
          message: "{payload.result}"
        baggage:
          healthdata.source: payload.source
          healthdata.result: payload.result
```

Review point: if both Garmin and Withings can route into the same lineage, use
source-specific ETL namespaces such as `garmin.etl.result` and
`withings.etl.result` instead of shared `healthdata.result`.

### `llm-pipeline`

Purpose: keep synchronous behavior while avoiding collision between YouTube and
web fetch paths. This pipeline needs a focused fixture before strict migration
because conditional steps may skip.

```yaml
  - name: llm-pipeline
    on: llm.request
    execution_mode: synchronous
    timeout: 120s
    steps:
      - id: transcript
        uses: youtube_transcript
        if:
          any:
            - path: payload.url
              op: contains
              value: youtube.com
            - path: payload.url
              op: contains
              value: youtu.be
        baggage:
          request.url: payload.url
          youtube.video_id: payload.video_id
          youtube.video_url: payload.video_url
          youtube.title: payload.title
          youtube.language: payload.language
      - id: fetch
        uses: jina-reader
        if:
          not:
            path: payload.video_id
            op: exists
        baggage:
          request.url: payload.url
          web.url: payload.url
          web.content_hash: payload.content_hash
          web.truncated: payload.truncated
      - id: summarize
        uses: fabric
        baggage:
          summary.text: payload.result
          summary.input_length: payload.input_length
          summary.output_length: payload.output_length
```

Review point: both `transcript` and `fetch` claim `request.url`. That is safe
only if they claim the same value and never both produce conflicting payloads in
one lineage. A focused fixture should prove this before applying.

## Single-Step Pipelines

These can remain in transition mode while multi-step pipelines migrate:

- `desk-presence-notify`
- `discord-ai`
- `github-interest-notify`
- `github-repo-sync-notify`
- `repo-maintenance-notify`
- `job-failure-notify`
- `claude-harvest-worker`
- `claude-harvest-notify`
- `ap-canary-registered`
- `apt-security-notify`
- `job-complete-notify`

If strict mode is desired for terminal notification/audit pipelines, add only
the durable facts the author wants to search or reason about later.

Possible examples:

```yaml
  - name: desk-presence-notify
    on: desk.presence.changed
    steps:
      - id: notify
        uses: discord_notify
        with:
          message: "{payload.message}"
        baggage:
          desk.presence.message: payload.message
```

```yaml
  - name: github-interest-notify
    on: github.interest.starred
    steps:
      - id: notify
        uses: github_interest_notify
        baggage:
          github.repository.full_name: payload.repository.full_name
          github.repository.stargazers_count: payload.repository.stargazers_count
          github.sender.login: payload.sender.login
```

```yaml
  - name: ap-canary-registered
    on: agent_handshake.registered
    steps:
      - id: notify
        uses: ap_canary_notify
        baggage:
          agent.email: payload.email
          agent.name: payload.agent
```

## Review Needed Before Applying

### Optional Claims

The current Sprint 3 implementation treats missing claim sources as errors. Do
not claim optional fields unless the event contract guarantees them. This
especially affects:

- `request.output_path`
- `request.filename`
- `file.output_path`
- playlist identity fields
- webhook metadata fields
- plugin debug fields such as `stderr`

Optional claim syntax is not part of Sprint 3 yet.

### Large Values

Do not make these durable by default:

- full transcript text
- full fetched web content
- command stdout/stderr
- raw webhook bodies

They can be made durable if there is a deliberate replay/audit requirement, but
they should not enter baggage just because they are available.

### Plugin Naming Cleanup

The config migration will be easier after plugin outputs use clearer names, but
that should not block Sprint 3. Keep the first pass author-side:

- map `payload.result` to domain names at `baggage:` boundaries
- add `with:` where downstream plugins expect `message` or `content`
- defer plugin output renames unless a plugin is actively confusing or brittle

## Suggested Order

1. Keep the live config unchanged while branch tests finish.
2. Add a non-prod fixture based on this proposal for three representative live
   shapes:
   - content pipeline: `youtube-wisdom` or `web-summarize`
   - repo pipeline: `repo-compliance`
   - notification pipeline: `astro-rebuild-staging-on-summary-change`
3. Confirm `with:` context interpolation behavior.
4. Apply strict `baggage:` to multi-step live pipelines in reviewed batches.
5. Leave single-step pipelines in transition until their durable audit names are
   worth claiming.

## Batch 1 Artifact

The first concrete live-config batch is prepared but not applied:

```text
.agent-notes/ductile-sprint-3-live-config-batch-1.md
.agent-notes/ductile-sprint-3-live-config-batch-1.proposed.yaml
```

Batch 1 covers:

- `astro-rebuild-staging-on-summary-change`
- `web-summarize`

It was validated against a copied config directory with `ductile config check`.
The only warnings were existing duplicate/unused plugin discovery warnings in
the live config environment.

## Expected Config Modification Burden

For the current live config:

- no immediate required edit while transition mode is enabled
- about 8-10 multi-step pipelines should get explicit baggage for strict mode
- single-step pipelines can mostly wait
- likely one or two `with:` additions are needed for notification QoL
- plugin/default namespace support is not required if authors own namespaces
