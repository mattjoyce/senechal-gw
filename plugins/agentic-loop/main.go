package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mattjoyce/ductile/internal/protocol"
)

const (
	defaultMaxLoops      = 10
	defaultMaxReframes   = 2
	defaultToolCommand   = "handle"
	defaultSkillsCommand = "ductile system skills"
)

type pluginConfig struct {
	MaxLoops           int
	MaxReframes        int
	AllowedPlugins     []string
	DefaultTool        string
	DefaultToolCommand string
	SkillsCommand      string
}

type runState struct {
	Status          string `json:"status"`
	Goal            string `json:"goal"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	Step            int    `json:"step"`
	MaxLoops        int    `json:"max_loops"`
	Reframes        int    `json:"reframes"`
	MaxReframes     int    `json:"max_reframes"`
	PendingStep     int    `json:"pending_step"`
	PendingTool     string `json:"pending_tool"`
	PendingSince    string `json:"pending_since"`
	LastToolCommand string `json:"last_tool_command"`
}

type pluginState struct {
	Runs      map[string]*runState `json:"runs"`
	LastRunID string               `json:"last_run_id,omitempty"`
}

type toolResult struct {
	RunID  string
	Step   int
	Tool   string
	Status string
	Result any
	Error  string
}

func main() {
	resp := handle()
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

func handle() protocol.Response {
	var req protocol.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		return errResp(fmt.Sprintf("invalid request JSON: %v", err), false)
	}

	cfg := parseConfig(req.Config)
	state := parseState(req.State)

	switch strings.TrimSpace(req.Command) {
	case "poll":
		return protocol.Response{
			Status: "ok",
			StateUpdates: map[string]any{
				"last_poll": nowISO(),
			},
			Logs: []protocol.LogEntry{
				info("agentic-loop poll noop"),
			},
		}
	case "health":
		return protocol.Response{
			Status: "ok",
			StateUpdates: map[string]any{
				"last_health_check":    nowISO(),
				"configured_max_loops": cfg.MaxLoops,
			},
			Logs: []protocol.LogEntry{
				info(fmt.Sprintf("healthy; tracked_runs=%d", len(state.Runs))),
			},
		}
	case "handle":
		if req.Event == nil {
			return errResp("handle requires event payload", false)
		}
		return handleEvent(req, cfg, state)
	default:
		return errResp(fmt.Sprintf("unknown command: %s", req.Command), false)
	}
}

func handleEvent(req protocol.Request, cfg pluginConfig, state pluginState) protocol.Response {
	event := *req.Event

	switch event.Type {
	case "agentic.start", "api.trigger":
		return handleStart(req, cfg, state, event)
	case "agentic.tool_result":
		result, err := parseToolResultEvent(event, req.Context)
		if err != nil {
			return errResp(err.Error(), false)
		}
		return handleToolResult(req, cfg, state, result)
	default:
		// Resume path for pipeline output events where correlation values live in context.
		if looksLikeResumeContext(req.Context) {
			result, err := parseToolResultEvent(event, req.Context)
			if err != nil {
				return errResp(err.Error(), false)
			}
			return handleToolResult(req, cfg, state, result)
		}
		return errResp(
			fmt.Sprintf("unsupported event type %q (expected agentic.start, api.trigger, agentic.tool_result)", event.Type),
			false,
		)
	}
}

func handleStart(req protocol.Request, cfg pluginConfig, state pluginState, event protocol.Event) protocol.Response {
	payload := event.Payload
	goal := asString(payload["goal"])
	if goal == "" {
		goal = asString(payload["text"])
	}
	if goal == "" {
		return errResp("agent start requires payload.goal", false)
	}

	contextMap := asMap(payload["context"])
	runID := asString(payload["run_id"])
	if runID == "" {
		runID = uuid.NewString()
	}

	tool, toolCommand, toolPayload := chooseFirstTool(goal, contextMap, payload, cfg)
	if !isAllowedTool(tool, cfg.AllowedPlugins) {
		return errResp(fmt.Sprintf("tool %q is not allowed by config.allowed_plugins", tool), false)
	}

	now := nowISO()
	state.Runs[runID] = &runState{
		Status:          "running",
		Goal:            goal,
		CreatedAt:       now,
		UpdatedAt:       now,
		Step:            1,
		MaxLoops:        cfg.MaxLoops,
		Reframes:        0,
		MaxReframes:     cfg.MaxReframes,
		PendingStep:     1,
		PendingTool:     tool,
		PendingSince:    now,
		LastToolCommand: toolCommand,
	}
	state.LastRunID = runID

	var logs []protocol.LogEntry
	logs = append(logs, info(fmt.Sprintf("started run %s", runID)))
	logs = append(logs, info(fmt.Sprintf("step=1 pending_tool=%s", tool)))

	if req.WorkspaceDir != "" {
		if err := os.MkdirAll(req.WorkspaceDir, 0o755); err != nil {
			logs = append(logs, warn(fmt.Sprintf("failed to create workspace: %v", err)))
		} else {
			initWorkspace(req.WorkspaceDir, goal, contextMap, runID, cfg, &logs)
			appendDecision(req.WorkspaceDir, "frame", "Initialized run, definition of done, and first action.", &logs)
			writePlan(req.WorkspaceDir, 1, tool, toolPayload, "Start with highest-signal tool.", &logs)
		}
	}

	eventOut := makeToolRequestEvent(runID, 1, tool, toolCommand, toolPayload)
	return okResp([]protocol.Event{eventOut}, stateToMap(state), logs)
}

func handleToolResult(req protocol.Request, cfg pluginConfig, state pluginState, result toolResult) protocol.Response {
	run := state.Runs[result.RunID]
	if run == nil {
		return okResp(nil, nil, []protocol.LogEntry{
			warn(fmt.Sprintf("unknown run_id=%s; ignoring", result.RunID)),
		})
	}

	if run.Status != "running" {
		return okResp(nil, nil, []protocol.LogEntry{
			info(fmt.Sprintf("run_id=%s already terminal (%s); ignoring", result.RunID, run.Status)),
		})
	}

	if result.Step < run.PendingStep {
		return okResp(nil, nil, []protocol.LogEntry{
			info(fmt.Sprintf("stale result run_id=%s step=%d pending=%d", result.RunID, result.Step, run.PendingStep)),
		})
	}

	if result.Step != run.PendingStep {
		return escalateRun(
			state, result.RunID, "step_mismatch",
			map[string]any{"expected_step": run.PendingStep, "actual_step": result.Step},
			fmt.Sprintf("protocol step mismatch (expected=%d got=%d)", run.PendingStep, result.Step),
		)
	}

	if result.Tool != run.PendingTool {
		return escalateRun(
			state, result.RunID, "pending_tool_mismatch",
			map[string]any{"expected_tool": run.PendingTool, "actual_tool": result.Tool},
			fmt.Sprintf("tool mismatch (expected=%s got=%s)", run.PendingTool, result.Tool),
		)
	}

	if strings.ToLower(result.Status) != "ok" {
		return escalateRun(
			state, result.RunID, "tool_error",
			map[string]any{"step": result.Step, "tool": result.Tool, "error": result.Error},
			fmt.Sprintf("tool returned non-ok status at step=%d", result.Step),
		)
	}

	// Per card #104 safety rule: if step >= max_loops, stop with escalation.
	if result.Step >= run.MaxLoops {
		return escalateRun(
			state, result.RunID, "max_loops_exceeded",
			map[string]any{"step": result.Step, "max_loops": run.MaxLoops},
			fmt.Sprintf("max_loops reached at step=%d", result.Step),
		)
	}

	var logs []protocol.LogEntry
	if req.WorkspaceDir != "" {
		appendReflection(req.WorkspaceDir, run, result, &logs)
	}

	if shouldFinish(run, result) {
		run.Status = "done"
		run.UpdatedAt = nowISO()
		run.PendingStep = 0
		run.PendingTool = ""
		state.LastRunID = result.RunID

		outcome := extractOutcome(result.Result)
		artifacts := extractArtifacts(result.Result)
		if req.WorkspaceDir != "" {
			markMemoryDone(req.WorkspaceDir, &logs)
			writePlanCompletion(req.WorkspaceDir, result.Step, &logs)
			appendDecision(req.WorkspaceDir, "reflect", "Goal appears complete; emitting agent.completed.", &logs)
		}

		completed := protocol.Event{
			Type: "agent.completed",
			Payload: map[string]any{
				"run_id":      result.RunID,
				"goal":        run.Goal,
				"outcome":     outcome,
				"steps_taken": result.Step,
				"artifacts":   artifacts,
			},
		}
		return okResp([]protocol.Event{completed}, stateToMap(state), append(logs, info(fmt.Sprintf("run %s completed", result.RunID))))
	}

	nextStep := result.Step + 1
	if nextStep >= run.MaxLoops {
		return escalateRun(
			state, result.RunID, "max_loops_exceeded",
			map[string]any{"step": nextStep, "max_loops": run.MaxLoops},
			fmt.Sprintf("next step would exceed max_loops (%d)", run.MaxLoops),
		)
	}

	nextTool, nextCommand, nextPayload := chooseNextAction(*run, result, cfg)
	if nextTool == "" {
		return escalateRun(
			state, result.RunID, "no_next_action",
			map[string]any{"step": result.Step},
			"planner did not produce a next action",
		)
	}
	if !isAllowedTool(nextTool, cfg.AllowedPlugins) {
		return escalateRun(
			state, result.RunID, "followup_tool_not_allowed",
			map[string]any{"tool": nextTool},
			fmt.Sprintf("follow-up tool %q not allowed", nextTool),
		)
	}

	run.Step = nextStep
	run.PendingStep = nextStep
	run.PendingTool = nextTool
	run.PendingSince = nowISO()
	run.LastToolCommand = nextCommand
	run.UpdatedAt = nowISO()

	if req.WorkspaceDir != "" {
		writePlan(req.WorkspaceDir, nextStep, nextTool, nextPayload, "Continue based on tool result.", &logs)
		appendDecision(req.WorkspaceDir, "plan", fmt.Sprintf("Planned next action: %s", nextTool), &logs)
	}

	requestEvent := makeToolRequestEvent(result.RunID, nextStep, nextTool, nextCommand, nextPayload)
	logs = append(logs, info(fmt.Sprintf("run %s advanced to step=%d tool=%s", result.RunID, nextStep, nextTool)))
	return okResp([]protocol.Event{requestEvent}, stateToMap(state), logs)
}

func parseToolResultEvent(event protocol.Event, ctx map[string]any) (toolResult, error) {
	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	// Normal path: explicit agentic.tool_result payload.
	if event.Type == "agentic.tool_result" {
		runID := asString(payload["run_id"])
		step := asInt(payload["step"], -1)
		tool := asString(payload["tool"])
		status := asString(payload["status"])
		if status == "" {
			status = "ok"
		}
		if runID == "" || step < 1 || tool == "" {
			return toolResult{}, fmt.Errorf("agentic.tool_result missing run_id/step/tool")
		}
		return toolResult{
			RunID:  runID,
			Step:   step,
			Tool:   tool,
			Status: status,
			Result: payload["result"],
			Error:  asString(payload["error"]),
		}, nil
	}

	// Resume path: tool event payload + correlation in protocol context.
	runID := asString(ctx["run_id"])
	step := asInt(ctx["step"], -1)
	tool := asString(ctx["tool"])
	if runID == "" || step < 1 || tool == "" {
		return toolResult{}, fmt.Errorf("resume context missing run_id/step/tool")
	}
	return toolResult{
		RunID:  runID,
		Step:   step,
		Tool:   tool,
		Status: "ok",
		Result: payload,
	}, nil
}

func chooseFirstTool(goal string, ctx map[string]any, startPayload map[string]any, cfg pluginConfig) (string, string, map[string]any) {
	tool := asString(ctx["tool"])
	if tool == "" {
		tool = asString(ctx["initial_tool"])
	}
	if tool == "" {
		tool = cfg.DefaultTool
	}

	command := asString(ctx["tool_command"])
	if command == "" {
		command = cfg.DefaultToolCommand
	}

	payload := cloneMap(asMap(ctx["tool_payload"]))
	if payload == nil {
		payload = map[string]any{}
	}

	if url := asString(startPayload["url"]); url != "" && payload["url"] == nil {
		payload["url"] = url
	}

	if tool == "" {
		if url := extractFirstURL(goal); url != "" {
			tool = "jina-reader"
			if payload["url"] == nil {
				payload["url"] = url
			}
		} else {
			tool = "fabric"
		}
	}

	if tool == "jina-reader" && payload["url"] == nil {
		if url := extractFirstURL(goal); url != "" {
			payload["url"] = url
		}
	}

	if tool == "fabric" && payload["text"] == nil && payload["prompt"] == nil {
		payload["prompt"] = goal
	}

	return tool, command, payload
}

func chooseNextAction(run runState, result toolResult, cfg pluginConfig) (string, string, map[string]any) {
	if result.Tool == "jina-reader" {
		text := extractBestText(result.Result)
		payload := map[string]any{
			"prompt": "Write a constructive two-paragraph critique of the supplied webpage content.",
		}
		if text != "" {
			payload["text"] = text
		}
		return "fabric", cfg.DefaultToolCommand, payload
	}

	// Default path: finalize after non-fetch tools.
	return "", "", nil
}

func shouldFinish(run *runState, result toolResult) bool {
	if done := asBoolFromResult(result.Result, "done"); done {
		return true
	}
	if done := asBoolFromResult(result.Result, "completed"); done {
		return true
	}
	return result.Tool == "fabric"
}

func makeToolRequestEvent(runID string, step int, tool, command string, toolPayload map[string]any) protocol.Event {
	payload := map[string]any{
		"run_id":       runID,
		"step":         step,
		"tool":         tool,
		"tool_command": command,
		"requested_at": nowISO(),
	}
	for k, v := range toolPayload {
		if _, reserved := payload[k]; reserved {
			continue
		}
		payload[k] = v
	}
	return protocol.Event{
		Type:      "agentic.tool_request." + tool,
		Payload:   payload,
		DedupeKey: fmt.Sprintf("agentic:run:%s:step:%d:request", runID, step),
	}
}

func escalateRun(state pluginState, runID, reason string, extra map[string]any, msg string) protocol.Response {
	run := state.Runs[runID]
	if run != nil {
		run.Status = "escalated"
		run.UpdatedAt = nowISO()
	}
	payload := map[string]any{
		"run_id": runID,
		"reason": reason,
	}
	for k, v := range extra {
		payload[k] = v
	}
	return okResp(
		[]protocol.Event{{Type: "agent.escalated", Payload: payload}},
		stateToMap(state),
		[]protocol.LogEntry{errLog(fmt.Sprintf("run %s escalated: %s", runID, msg))},
	)
}

func initWorkspace(dir, goal string, ctx map[string]any, runID string, cfg pluginConfig, logs *[]protocol.LogEntry) {
	_ = writeFile(filepath.Join(dir, "context.md"), buildContextMD(goal, ctx))
	_ = writeFile(filepath.Join(dir, "memory.md"), buildMemoryMD())
	_ = writeFile(filepath.Join(dir, "plan.md"), "# Plan\n\n- Pending first action.\n")
	_ = writeFile(filepath.Join(dir, "decisions.md"), fmt.Sprintf("# Decisions\n\n## %s\n- Run initialized (%s)\n", nowISO(), runID))
	_ = os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755)

	skills, err := generateSkillsMarkdown(cfg.SkillsCommand)
	if err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("skills generation failed: %v", err)))
		skills = "# Skills\n\nFailed to run skill discovery command.\n"
	}
	_ = writeFile(filepath.Join(dir, "skills.md"), skills)
}

func writePlan(dir string, step int, tool string, payload map[string]any, rationale string, logs *[]protocol.LogEntry) {
	content := fmt.Sprintf(
		"# Plan\n\n## Step List\n- [ ] Step %d\n\n## Next Action\n- tool: `%s`\n- payload: `%s`\n\n## Rationale\n%s\n",
		step, tool, compactJSON(payload), rationale,
	)
	if err := writeFile(filepath.Join(dir, "plan.md"), content); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("write plan.md failed: %v", err)))
	}
}

func writePlanCompletion(dir string, step int, logs *[]protocol.LogEntry) {
	content := fmt.Sprintf("# Plan\n\n- [x] Completed through step %d\n- Finalized run\n", step)
	if err := writeFile(filepath.Join(dir, "plan.md"), content); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("write completed plan.md failed: %v", err)))
	}
}

func appendReflection(dir string, run *runState, result toolResult, logs *[]protocol.LogEntry) {
	entry := fmt.Sprintf(
		"\n## %s\n- tool: %s\n- step: %d\n- summary: %s\n",
		nowISO(),
		result.Tool,
		result.Step,
		shorten(extractOutcome(result.Result), 500),
	)
	if err := appendFile(filepath.Join(dir, "decisions.md"), entry); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("append decisions.md failed: %v", err)))
	}
	memEntry := fmt.Sprintf(
		"\n## Learned Fact (%s)\n- %s\n",
		nowISO(),
		shorten(extractBestText(result.Result), 500),
	)
	if err := appendFile(filepath.Join(dir, "memory.md"), memEntry); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("append memory.md failed: %v", err)))
	}
}

func markMemoryDone(dir string, logs *[]protocol.LogEntry) {
	entry := "\n## Status\n- [x] Goal completed.\n"
	if err := appendFile(filepath.Join(dir, "memory.md"), entry); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("append completion to memory.md failed: %v", err)))
	}
}

func appendDecision(dir, phase, text string, logs *[]protocol.LogEntry) {
	entry := fmt.Sprintf("\n## %s (%s)\n- %s\n", nowISO(), phase, text)
	if err := appendFile(filepath.Join(dir, "decisions.md"), entry); err != nil {
		*logs = append(*logs, warn(fmt.Sprintf("append decisions.md failed: %v", err)))
	}
}

func buildContextMD(goal string, ctx map[string]any) string {
	return fmt.Sprintf(
		"# Goal\n\n%s\n\n## Context\n\n```json\n%s\n```\n",
		goal,
		prettyJSON(ctx),
	)
}

func buildMemoryMD() string {
	return `# Working Memory

## Definition of Done
- [ ] Clarify objective and constraints from the goal.
- [ ] Gather required source information with at least one tool call.
- [ ] Produce a final operator-facing output.

## Learned Facts
- (none yet)

## Status
- Run initialized.
`
}

func generateSkillsMarkdown(command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		command = defaultSkillsCommand
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run %q: %w (%s)", command, err, strings.TrimSpace(string(out)))
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		text = "(no skills returned)"
	}
	return fmt.Sprintf("# Skills\n\nGenerated from `%s` on %s.\n\n```\n%s\n```\n", command, nowISO(), text), nil
}

func parseConfig(cfg map[string]any) pluginConfig {
	out := pluginConfig{
		MaxLoops:           defaultMaxLoops,
		MaxReframes:        defaultMaxReframes,
		DefaultToolCommand: defaultToolCommand,
		SkillsCommand:      defaultSkillsCommand,
	}
	if cfg == nil {
		return out
	}

	if v := asInt(cfg["max_loops"], 0); v > 0 {
		out.MaxLoops = v
	} else if v := asInt(cfg["max_steps"], 0); v > 0 {
		out.MaxLoops = v
	}
	if v := asInt(cfg["max_reframes"], 0); v > 0 {
		out.MaxReframes = v
	}
	if v := asString(cfg["default_tool"]); v != "" {
		out.DefaultTool = v
	}
	if v := asString(cfg["default_tool_command"]); v != "" {
		out.DefaultToolCommand = v
	}
	if v := asString(cfg["skills_command"]); v != "" {
		out.SkillsCommand = v
	}
	out.AllowedPlugins = asStringSlice(cfg["allowed_plugins"])
	return out
}

func parseState(in map[string]any) pluginState {
	out := pluginState{Runs: map[string]*runState{}}
	if in == nil {
		return out
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out.Runs == nil {
		out.Runs = map[string]*runState{}
	}
	for _, run := range out.Runs {
		if run == nil {
			continue
		}
		if run.MaxLoops <= 0 {
			run.MaxLoops = defaultMaxLoops
		}
		if run.MaxReframes <= 0 {
			run.MaxReframes = defaultMaxReframes
		}
	}
	return out
}

func stateToMap(state pluginState) map[string]any {
	raw, err := json.Marshal(state)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func looksLikeResumeContext(ctx map[string]any) bool {
	if ctx == nil {
		return false
	}
	return asString(ctx["run_id"]) != "" && asInt(ctx["step"], -1) > 0 && asString(ctx["tool"]) != ""
}

func isAllowedTool(tool string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.TrimSpace(item) == tool {
			return true
		}
	}
	return false
}

func extractOutcome(result any) string {
	if s, ok := result.(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	m := asMap(result)
	if m == nil {
		return "Run completed."
	}
	for _, key := range []string{"result", "summary", "content", "text"} {
		if s := asString(m[key]); s != "" {
			return s
		}
	}
	return "Run completed."
}

func extractArtifacts(result any) []string {
	m := asMap(result)
	if m == nil {
		return nil
	}
	var out []string
	for _, key := range []string{"artifact_path", "output_path"} {
		if s := asString(m[key]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func extractBestText(result any) string {
	m := asMap(result)
	if m == nil {
		return extractOutcome(result)
	}
	for _, key := range []string{"text", "content", "excerpt", "result", "summary"} {
		if s := asString(m[key]); s != "" {
			return s
		}
	}
	return ""
}

func asBoolFromResult(result any, key string) bool {
	m := asMap(result)
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

var urlPattern = regexp.MustCompile(`https?://[^\s)]+`)

func extractFirstURL(text string) string {
	match := urlPattern.FindString(text)
	return strings.TrimSpace(match)
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func info(msg string) protocol.LogEntry {
	return protocol.LogEntry{Level: "info", Message: msg}
}

func warn(msg string) protocol.LogEntry {
	return protocol.LogEntry{Level: "warn", Message: msg}
}

func errLog(msg string) protocol.LogEntry {
	return protocol.LogEntry{Level: "error", Message: msg}
}

func okResp(events []protocol.Event, state map[string]any, logs []protocol.LogEntry) protocol.Response {
	resp := protocol.Response{Status: "ok"}
	if len(events) > 0 {
		resp.Events = events
	}
	if len(state) > 0 {
		resp.StateUpdates = state
	}
	if len(logs) > 0 {
		resp.Logs = logs
	}
	return resp
}

func errResp(message string, retry bool) protocol.Response {
	retryVal := retry
	return protocol.Response{
		Status: "error",
		Error:  message,
		Retry:  &retryVal,
		Logs:   []protocol.LogEntry{errLog(message)},
	}
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return ""
	}
}

func asInt(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		if t > 0 {
			return t
		}
	case int64:
		if t > 0 {
			return int(t)
		}
	case float64:
		if int(t) > 0 {
			return int(t)
		}
	case json.Number:
		if i, err := t.Int64(); err == nil && i > 0 {
			return int(i)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(t, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func asStringSlice(v any) []string {
	var out []string
	switch t := v.(type) {
	case []string:
		for _, item := range t {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
	case []any:
		for _, item := range t {
			if s := asString(item); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func prettyJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func compactJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max < 4 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
