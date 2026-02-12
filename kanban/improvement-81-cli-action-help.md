---
id: 81
status: done
priority: Medium
tags: [improvement, cli, ux, discoverability, llm-operator]
---

# IMPROVEMENT: Add --help support at action level

## Description

CLI commands don't support `--help` at the action level. Users cannot discover flags for specific commands without reading source code or documentation. This was reported in card #74 and remained unfixed.

## Impact

- **Severity**: Medium - UX and discoverability issue
- **Scope**: All CLI actions
- **User Experience**: Poor discoverability, trial-and-error usage

## Current Behavior

```bash
# Attempt to get help for specific command
$ ./senechal-gw config check --help
Unknown config action: --help
❌ Treats --help as an action name

# Noun-level help works
$ ./senechal-gw config help
Usage: senechal-gw config <action> [--config PATH]
Actions: lock, check, show, get, set
⚠️ Lists actions but not flags

# No way to discover flags for 'config check'
# User must guess or read source code
```

## Expected Behavior

**Action-level help should work:**

```bash
# Get help for specific command
$ ./senechal-gw config check --help

Usage: senechal-gw config check [flags]

Validate configuration files for syntax, policy, and integrity.

Flags:
  --config PATH     Path to config file or directory (default: current dir)
  --format FORMAT   Output format: human, json (default: human)
  --strict          Treat warnings as errors (exit code 2)

Examples:
  # Check current directory config
  senechal-gw config check

  # Check specific config
  senechal-gw config check --config /etc/senechal-gw

  # JSON output for scripting
  senechal-gw config check --format json

  # Fail on warnings
  senechal-gw config check --strict

Exit Codes:
  0  Configuration valid
  1  Configuration invalid (errors)
  2  Configuration has warnings (--strict mode only)
```

**All commands should support --help:**

```bash
# Config commands
./senechal-gw config check --help
./senechal-gw config show --help
./senechal-gw config get --help
./senechal-gw config set --help
./senechal-gw config lock --help

# Job commands
./senechal-gw job inspect --help

# System commands
./senechal-gw system start --help
```

## Affected Commands

**Config actions:**
- `config check` - Need: --config, --format, --strict
- `config show` - Need: --config
- `config get` - Need: --config, <path>
- `config set` - Need: --config, --dry-run, --apply, <path>=<value>
- `config lock` - Need: --config, -v/--verbose

**Job actions:**
- `job inspect` - Need: --config, --json, <job_id>

**System actions:**
- `system start` - Need: --config, --log-level, flags

## Implementation Approach

**Option 1: Standard flag library support**
```go
import "flag"

func runConfigCheck(args []string) error {
    fs := flag.NewFlagSet("config check", flag.ExitOnError)

    // This automatically handles --help
    config := fs.String("config", ".", "Path to config file")
    format := fs.String("format", "human", "Output format (human|json)")
    strict := fs.Bool("strict", false, "Treat warnings as errors")

    fs.Parse(args)

    // Command implementation
}
```

**Option 2: Cobra framework (recommended)**
```go
import "github.com/spf13/cobra"

var checkCmd = &cobra.Command{
    Use:   "check",
    Short: "Validate configuration files",
    Long: `Validate configuration files for syntax, policy, and integrity.

Checks include:
  - YAML syntax validation
  - Required fields presence
  - Plugin schedule validation
  - Config integrity hash verification
`,
    Example: `  # Check current directory
  senechal-gw config check

  # Check specific config
  senechal-gw config check --config /etc/senechal-gw

  # JSON output for scripting
  senechal-gw config check --format json`,
    Run: func(cmd *cobra.Command, args []string) {
        // Command implementation
    },
}

func init() {
    checkCmd.Flags().String("config", ".", "Path to config file")
    checkCmd.Flags().String("format", "human", "Output format (human|json)")
    checkCmd.Flags().Bool("strict", false, "Treat warnings as errors")
}
```

## Benefits

**For users:**
1. ✅ Self-documenting CLI
2. ✅ No need to read external docs for basic usage
3. ✅ Examples show correct syntax
4. ✅ Flag descriptions explain purpose
5. ✅ Discoverability without trial-and-error

**For LLM operators:**
1. ✅ Can query help text programmatically
2. ✅ Learn correct flag syntax
3. ✅ Understand exit codes
4. ✅ See examples for context

**For developers:**
1. ✅ Automatic help generation from code
2. ✅ Consistency across commands
3. ✅ Less documentation maintenance

## Related

- **Card #74** - Originally reported this issue
- **Card #38** - CLI config management spec (includes help text requirements)

## Priority

**Medium** - Not blocking functionality, but significantly improves UX and discoverability for both humans and LLM operators.

## Testing Recommendations

After implementation, verify:
1. ✅ All actions support `--help` flag
2. ✅ Help text includes flag descriptions
3. ✅ Help text includes examples
4. ✅ Help text explains exit codes
5. ✅ `-h` short form also works
6. ✅ Help output is properly formatted (wrap at 80 cols)
7. ✅ Examples are copy-pasteable

## Narrative

- 2026-02-12: Re-confirmed during comprehensive CLI testing. This issue was originally reported in card #74 but remains unfixed. Running `./senechal-gw config check --help` produces "Unknown config verb: --help" because the parser treats --help as a verb name. The noun-level help (`config help`) works but only lists verb names without flag documentation. Users must guess flags or read source code. This is a discoverability issue affecting both human users and LLM operators. Recommendation: Use Cobra framework for automatic --help generation or implement manual help handler that recognizes --help flag before verb parsing. Medium priority - doesn't block functionality but significantly impacts UX. (by @test-admin)
- 2026-02-12: Implemented action-level `--help`/`-h` handling across noun dispatchers and renamed user-facing CLI terminology from `verb` to `action` in command usage and errors. Added CLI tests for `config`, `job`, and `system` action help output. (by @assistant)
