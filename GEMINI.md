# Gemini's Learnings from Agent 3 Task: Scheduler Tick Loop and Crash Recovery

During the implementation of the scheduler tick loop and crash recovery, several valuable lessons were reinforced regarding precision in tooling, understanding environment configurations, and the nuances of testing.

## Lessons Learned:

### 1. Precision with the `replace` Tool
**Struggle:** I repeatedly provided `old_string` values that were too generic (e.g., a lone `}`). This led to the `replace` tool failing because it either found multiple occurrences or no exact match, requiring multiple attempts to correct.
**Lesson:** When using the `replace` tool, the `old_string` parameter *must be highly specific*. It should include sufficient surrounding context (at least 3 lines before and after, matching whitespace exactly) to uniquely identify the target code block. This ensures precise, idempotent modifications and prevents unintended changes.

### 2. Go Module Import Paths and Environment Setup
**Struggle:** I encountered compilation errors due to incorrect Go package import paths (e.g., `ductile/internal/config` instead of the full `github.com/mattjoyce/ductile/internal/config`). This highlighted a misunderstanding of how Go modules resolve paths within a project. The issue was compounded when generating mock files, which also required the correct fully qualified import paths.
**Lesson:** Always verify the Go module path defined in `go.mod`. All internal package imports throughout the codebase (including test files and generated mocks) *must* use this full, correct module path (e.g., `module_name/path/to/package`). Pay close attention to this, especially after initial project setup or when adding new packages and generating code.

### 3. Effective `slog` Logger Testing
**Struggle:** My initial approach to mocking or testing components that use `log/slog` was flawed. I incorrectly assumed a `log.NewTestLogger()` function existed and then struggled with asserting against log output from a global logger instance, leading to unpredictable test results.
**Lesson:** When testing components that utilize `log/slog`, the most robust approach is to:
    a.  **Inject the logger:** Design components to accept an `*slog.Logger` instance (or a custom interface wrapping it) via dependency injection, rather than relying on global `log.Get()`.
    b.  **Capture output:** For tests, create a custom `slog.Handler` (such as the `TestLogBuffer` implemented) that directs log output to an in-memory buffer. This allows for programmatic inspection and assertion of log messages.

### 4. Precision in Test Assertions and Logic Flow
**Struggle:** In `TestSchedulerTick`, my assertions were initially imprecise due to a misinterpretation of the simplified MVP scheduling logic and how log messages were being captured. Assertions failed because a log message intended for one plugin was unexpectedly present when testing another, or because the underlying code flow led to different log outputs than initially assumed.
**Lesson:** Thoroughly understand the exact code path, including all conditional logic and potential error branches, and the expected outputs (including specific log messages and their attributes) before writing assertions. When asserting against aggregated log output, it's crucial to identify unique strings or patterns associated with specific events to avoid false positives or negatives. If possible, consider parsing log lines or writing more fine-grained tests to isolate log events. Re-evaluating the underlying logic of the code under test is paramount when assertions fail unexpectedly.

These experiences underscore the critical importance of meticulous attention to detail in coding, robust testing practices, and a deep understanding of the chosen language's module and logging conventions. For further guidance on project conventions and team collaboration, refer to `TEAM.md` and `COORDINATION.md`.