// Package configsnapshot records durable identities for active runtime configs.
package configsnapshot

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/zeebo/blake3"
	"gopkg.in/yaml.v3"
)

const (
	// SnapshotFormat is the serialization version for config_snapshots payloads.
	SnapshotFormat = 1

	ReasonStartup = "startup"
	ReasonReload  = "reload"
)

// Snapshot is one successful active runtime config value.
type Snapshot struct {
	ID                 string
	ConfigHash         string
	SourceHash         *string
	SourcePath         *string
	Source             *string
	Reason             string
	LoadedAt           time.Time
	DuctileVersion     *string
	BinaryPath         *string
	SnapshotFormat     int
	Semantics          json.RawMessage
	PluginFingerprints json.RawMessage
	SanitizedConfig    json.RawMessage
	SecretFingerprints json.RawMessage
}

// SecretUse records secret provenance and non-reversible identity.
type SecretUse struct {
	Purpose     string `json:"purpose"`
	Ref         string `json:"ref,omitempty"`
	Source      string `json:"source,omitempty"`
	Present     bool   `json:"present"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// PluginFingerprintRecord captures current plugin identity availability.
type PluginFingerprintRecord struct {
	Plugin                 string `json:"plugin"`
	Enabled                bool   `json:"enabled"`
	Uses                   string `json:"uses,omitempty"`
	Available              bool   `json:"available"`
	UnavailableReason      string `json:"unavailable_reason,omitempty"`
	ManifestPath           string `json:"manifest_path,omitempty"`
	ManifestResolvedPath   string `json:"manifest_resolved_path,omitempty"`
	ManifestHash           string `json:"manifest_hash,omitempty"`
	EntrypointPath         string `json:"entrypoint_path,omitempty"`
	EntrypointResolvedPath string `json:"entrypoint_resolved_path,omitempty"`
	EntrypointHash         string `json:"entrypoint_hash,omitempty"`
}

// PluginFingerprintRecordFromLock converts a verified lock entry into snapshot JSON shape.
func PluginFingerprintRecordFromLock(fp config.PluginFingerprint) PluginFingerprintRecord {
	return PluginFingerprintRecord{
		Plugin:                 fp.Name,
		Enabled:                fp.Enabled,
		Uses:                   fp.Uses,
		Available:              true,
		ManifestPath:           fp.ManifestPath,
		ManifestResolvedPath:   fp.ManifestResolvedPath,
		ManifestHash:           fp.ManifestHash,
		EntrypointPath:         fp.EntrypointPath,
		EntrypointResolvedPath: fp.EntrypointResolvedPath,
		EntrypointHash:         fp.EntrypointHash,
	}
}

// BuildInput contains runtime facts needed to create a snapshot.
type BuildInput struct {
	Config             *config.Config
	ConfigPath         string
	ConfigSource       string
	Reason             string
	DuctileVersion     string
	BinaryPath         string
	PluginFingerprints []PluginFingerprintRecord
	LoadedAt           time.Time
}

// Build creates an in-memory snapshot from the active runtime config.
func Build(input BuildInput) (*Snapshot, error) {
	if input.Config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if input.Reason != ReasonStartup && input.Reason != ReasonReload {
		return nil, fmt.Errorf("unsupported config snapshot reason %q", input.Reason)
	}
	loadedAt := input.LoadedAt.UTC()
	if loadedAt.IsZero() {
		loadedAt = time.Now().UTC()
	}

	sanitized, secretUses := sanitizeConfig(input.Config)
	semantics := currentSemantics()

	sanitizedJSON, err := canonicalJSON(sanitized)
	if err != nil {
		return nil, fmt.Errorf("marshal sanitized config: %w", err)
	}
	secretsJSON, err := canonicalJSON(secretUses)
	if err != nil {
		return nil, fmt.Errorf("marshal secret fingerprints: %w", err)
	}
	semanticsJSON, err := canonicalJSON(semantics)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime semantics: %w", err)
	}
	pluginFingerprintsJSON, err := canonicalJSON(input.PluginFingerprints)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin fingerprints: %w", err)
	}

	configHash, err := hashCanonical(map[string]any{
		"sanitized_config":    sanitized,
		"secret_fingerprints": secretUses,
		"snapshot_format":     SnapshotFormat,
	})
	if err != nil {
		return nil, fmt.Errorf("hash effective config: %w", err)
	}

	sourceHash, err := sourceHash(input.Config.SourceFiles)
	if err != nil {
		return nil, err
	}

	snapshotID := "cfg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sourcePath := strings.TrimSpace(input.ConfigPath)
	source := strings.TrimSpace(input.ConfigSource)
	version := strings.TrimSpace(input.DuctileVersion)
	binaryPath := strings.TrimSpace(input.BinaryPath)

	snapshot := &Snapshot{
		ID:                 snapshotID,
		ConfigHash:         configHash,
		Reason:             input.Reason,
		LoadedAt:           loadedAt,
		SnapshotFormat:     SnapshotFormat,
		Semantics:          semanticsJSON,
		PluginFingerprints: pluginFingerprintsJSON,
		SanitizedConfig:    sanitizedJSON,
		SecretFingerprints: secretsJSON,
	}
	if sourceHash != "" {
		snapshot.SourceHash = &sourceHash
	}
	if sourcePath != "" {
		snapshot.SourcePath = &sourcePath
	}
	if source != "" {
		snapshot.Source = &source
	}
	if version != "" {
		snapshot.DuctileVersion = &version
	}
	if binaryPath != "" {
		snapshot.BinaryPath = &binaryPath
	}
	return snapshot, nil
}

// Insert stores a snapshot. Snapshot IDs are UUID-backed, so conflicts are not expected.
func Insert(ctx context.Context, db *sql.DB, snapshot *Snapshot) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO config_snapshots(
  id, config_hash, source_hash, source_path, source, reason, loaded_at,
  ductile_version, binary_path, snapshot_format, semantics, plugin_fingerprints,
  sanitized_config, secret_fingerprints
)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, snapshot.ID, snapshot.ConfigHash, snapshot.SourceHash, snapshot.SourcePath, snapshot.Source,
		snapshot.Reason, snapshot.LoadedAt.UTC().Format(time.RFC3339Nano), snapshot.DuctileVersion,
		snapshot.BinaryPath, snapshot.SnapshotFormat, nullableJSON(snapshot.Semantics),
		nullableJSON(snapshot.PluginFingerprints), nullableJSON(snapshot.SanitizedConfig),
		nullableJSON(snapshot.SecretFingerprints))
	if err != nil {
		return fmt.Errorf("insert config snapshot: %w", err)
	}
	return nil
}

// Get loads a snapshot by ID.
func Get(ctx context.Context, db *sql.DB, id string) (*Snapshot, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("config snapshot id is empty")
	}
	var (
		snapshot           Snapshot
		loadedAtS          string
		sourceHash         sql.NullString
		sourcePath         sql.NullString
		source             sql.NullString
		version            sql.NullString
		binaryPath         sql.NullString
		semantics          sql.NullString
		pluginFingerprints sql.NullString
		sanitizedConfig    sql.NullString
		secretFingerprints sql.NullString
	)
	err := db.QueryRowContext(ctx, `
SELECT id, config_hash, source_hash, source_path, source, reason, loaded_at,
       ductile_version, binary_path, snapshot_format, semantics, plugin_fingerprints,
       sanitized_config, secret_fingerprints
FROM config_snapshots
WHERE id = ?;
`, id).Scan(&snapshot.ID, &snapshot.ConfigHash, &sourceHash, &sourcePath, &source, &snapshot.Reason,
		&loadedAtS, &version, &binaryPath, &snapshot.SnapshotFormat, &semantics,
		&pluginFingerprints, &sanitizedConfig, &secretFingerprints)
	if err != nil {
		return nil, fmt.Errorf("load config snapshot %s: %w", id, err)
	}
	loadedAt, err := time.Parse(time.RFC3339Nano, loadedAtS)
	if err != nil {
		return nil, fmt.Errorf("parse config snapshot loaded_at: %w", err)
	}
	snapshot.LoadedAt = loadedAt
	if sourceHash.Valid {
		snapshot.SourceHash = &sourceHash.String
	}
	if sourcePath.Valid {
		snapshot.SourcePath = &sourcePath.String
	}
	if source.Valid {
		snapshot.Source = &source.String
	}
	if version.Valid {
		snapshot.DuctileVersion = &version.String
	}
	if binaryPath.Valid {
		snapshot.BinaryPath = &binaryPath.String
	}
	if semantics.Valid {
		snapshot.Semantics = json.RawMessage(semantics.String)
	}
	if pluginFingerprints.Valid {
		snapshot.PluginFingerprints = json.RawMessage(pluginFingerprints.String)
	}
	if sanitizedConfig.Valid {
		snapshot.SanitizedConfig = json.RawMessage(sanitizedConfig.String)
	}
	if secretFingerprints.Valid {
		snapshot.SecretFingerprints = json.RawMessage(secretFingerprints.String)
	}
	return &snapshot, nil
}

func sanitizeConfig(cfg *config.Config) (map[string]any, []SecretUse) {
	secretUses := make([]SecretUse, 0)

	apiTokens := make([]map[string]any, 0, len(cfg.API.Auth.Tokens))
	for i, token := range cfg.API.Auth.Tokens {
		purpose := fmt.Sprintf("api.auth.tokens[%d].token", i)
		secretUses = append(secretUses, secretUse(purpose, "", "config", token.Token))
		apiTokens = append(apiTokens, map[string]any{
			"token":  "[redacted:" + purpose + "]",
			"scopes": append([]string(nil), token.Scopes...),
		})
	}

	tokenByName := make(map[string]string, len(cfg.Tokens))
	tokens := make([]map[string]any, 0, len(cfg.Tokens))
	for _, token := range cfg.Tokens {
		tokenByName[token.Name] = token.Key
		purpose := "tokens." + token.Name + ".key"
		secretUses = append(secretUses, secretUse(purpose, token.Name, "tokens.yaml", token.Key))
		tokens = append(tokens, map[string]any{
			"name":        token.Name,
			"key":         "[redacted:" + purpose + "]",
			"scopes_file": token.ScopesFile,
			"scopes_hash": token.ScopesHash,
			"created_at":  token.CreatedAt,
			"description": token.Description,
		})
	}

	webhooks := map[string]any(nil)
	if cfg.Webhooks != nil {
		endpoints := make([]map[string]any, 0, len(cfg.Webhooks.Endpoints))
		for _, endpoint := range cfg.Webhooks.Endpoints {
			if endpoint.SecretRef != "" {
				value, present := tokenByName[endpoint.SecretRef]
				purpose := "webhooks." + endpoint.Name + ".secret_ref"
				if endpoint.Name == "" {
					purpose = "webhooks." + endpoint.Path + ".secret_ref"
				}
				secretUses = append(secretUses, secretUsePresent(purpose, endpoint.SecretRef, "tokens.yaml", value, present))
			}
			endpoints = append(endpoints, map[string]any{
				"name":             endpoint.Name,
				"path":             endpoint.Path,
				"plugin":           endpoint.Plugin,
				"secret_ref":       endpoint.SecretRef,
				"signature_header": endpoint.SignatureHeader,
				"max_body_size":    endpoint.MaxBodySize,
			})
		}
		webhooks = map[string]any{
			"listen":    cfg.Webhooks.Listen,
			"endpoints": endpoints,
		}
	}

	plugins := make(map[string]any, len(cfg.Plugins))
	pluginNames := make([]string, 0, len(cfg.Plugins))
	for name := range cfg.Plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)
	for _, name := range pluginNames {
		pluginConf := cfg.Plugins[name]
		redactedConfig, pluginSecrets := sanitizePluginConfig("plugins."+name+".config", pluginConf.Config)
		secretUses = append(secretUses, pluginSecrets...)
		plugins[name] = map[string]any{
			"enabled":               pluginConf.Enabled,
			"uses":                  pluginConf.Uses,
			"schedules":             renderSchedules(pluginConf.NormalizedSchedules()),
			"retry":                 renderRetry(pluginConf.Retry),
			"timeouts":              renderTimeouts(pluginConf.Timeouts),
			"circuit_breaker":       renderCircuitBreaker(pluginConf.CircuitBreaker),
			"max_outstanding_polls": pluginConf.MaxOutstandingPolls,
			"parallelism":           pluginConf.Parallelism,
			"notify_on_complete":    pluginConf.NotifyOnComplete,
			"config":                redactedConfig,
		}
	}

	sort.Slice(secretUses, func(i, j int) bool {
		if secretUses[i].Purpose == secretUses[j].Purpose {
			return secretUses[i].Ref < secretUses[j].Ref
		}
		return secretUses[i].Purpose < secretUses[j].Purpose
	})

	return map[string]any{
		"service": map[string]any{
			"name":              cfg.Service.Name,
			"tick_interval":     durationString(cfg.Service.TickInterval),
			"log_level":         cfg.Service.LogLevel,
			"log_format":        cfg.Service.LogFormat,
			"dedupe_ttl":        durationString(cfg.Service.DedupeTTL),
			"job_log_retention": durationString(cfg.Service.JobLogRetention),
			"max_workers":       cfg.Service.MaxWorkers,
			"strict_mode":       cfg.Service.StrictMode,
			"allow_symlinks":    cfg.Service.AllowSymlinks,
		},
		"state": map[string]any{
			"path": cfg.State.Path,
		},
		"api": map[string]any{
			"enabled":             cfg.API.Enabled,
			"listen":              cfg.API.Listen,
			"auth":                map[string]any{"tokens": apiTokens},
			"max_concurrent_sync": cfg.API.MaxConcurrentSync,
			"max_sync_timeout":    durationString(cfg.API.MaxSyncTimeout),
		},
		"plugin_roots": append([]string(nil), cfg.PluginRoots...),
		"plugins":      plugins,
		"workspace": map[string]any{
			"ttl":              durationString(cfg.Workspace.TTL),
			"janitor_interval": durationString(cfg.Workspace.JanitorInterval),
		},
		"webhooks":  webhooks,
		"tokens":    tokens,
		"pipelines": renderPipelines(cfg.Pipelines),
	}, secretUses
}

func sanitizePluginConfig(path string, value any) (any, []SecretUse) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		var secrets []SecretUse
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "." + key
			if isSecretKey(key) {
				raw := fmt.Sprint(v[key])
				out[key] = "[redacted:" + childPath + "]"
				secrets = append(secrets, secretUse(childPath, "", "plugin_config", raw))
				continue
			}
			child, childSecrets := sanitizePluginConfig(childPath, v[key])
			out[key] = child
			secrets = append(secrets, childSecrets...)
		}
		return out, secrets
	case map[string]string:
		out := make(map[string]any, len(v))
		var secrets []SecretUse
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "." + key
			if isSecretKey(key) {
				out[key] = "[redacted:" + childPath + "]"
				secrets = append(secrets, secretUse(childPath, "", "plugin_config", v[key]))
				continue
			}
			out[key] = v[key]
		}
		return out, secrets
	case []any:
		out := make([]any, 0, len(v))
		var secrets []SecretUse
		for i, elem := range v {
			child, childSecrets := sanitizePluginConfig(fmt.Sprintf("%s[%d]", path, i), elem)
			out = append(out, child)
			secrets = append(secrets, childSecrets...)
		}
		return out, secrets
	default:
		return v, nil
	}
}

func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	secretNames := []string{"token", "api_key", "secret", "password", "credential", "private_key"}
	for _, name := range secretNames {
		if normalized == name || strings.Contains(normalized, name) {
			return true
		}
	}
	return false
}

func secretUse(purpose, ref, source, value string) SecretUse {
	return secretUsePresent(purpose, ref, source, value, strings.TrimSpace(value) != "")
}

func secretUsePresent(purpose, ref, source, value string, present bool) SecretUse {
	use := SecretUse{
		Purpose: purpose,
		Ref:     ref,
		Source:  source,
		Present: present,
	}
	if present {
		use.Fingerprint = secretFingerprint(purpose, value)
	}
	return use
}

func secretFingerprint(purpose, value string) string {
	sum := blake3.Sum256([]byte("ductile config secret fingerprint v1\x00" + purpose + "\x00" + value))
	return "secretfp_blake3:" + hex.EncodeToString(sum[:])
}

func currentSemantics() map[string]any {
	return map[string]any{
		"baggage_immutability": "origin_keys_only",
		"plugin_retry_field":   "v2_authoritative",
		"retry_policy_owner":   "plugin_response_plus_core_config",
	}
}

func sourceHash(sourceFiles map[string]*yaml.Node) (string, error) {
	paths := make([]string, 0, len(sourceFiles))
	for path := range sourceFiles {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return "", nil
	}
	sort.Strings(paths)
	hasher := blake3.New()
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		// #nosec G304 -- config source paths are operator-controlled local inputs.
		data, err := os.ReadFile(cleaned)
		if err != nil {
			return "", fmt.Errorf("hash config source %s: %w", cleaned, err)
		}
		_, _ = hasher.Write([]byte(cleaned))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(data)
		_, _ = hasher.Write([]byte{0})
	}
	return "blake3:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashCanonical(value any) (string, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := blake3.Sum256(data)
	return "blake3:" + hex.EncodeToString(sum[:]), nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return string(data)
}

func durationString(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func renderRetry(retry *config.RetryConfig) any {
	if retry == nil {
		return nil
	}
	return map[string]any{
		"max_attempts": retry.MaxAttempts,
		"backoff_base": durationString(retry.BackoffBase),
	}
}

func renderTimeouts(timeouts *config.TimeoutsConfig) any {
	if timeouts == nil {
		return nil
	}
	return map[string]any{
		"poll":   durationString(timeouts.Poll),
		"handle": durationString(timeouts.Handle),
		"health": durationString(timeouts.Health),
		"init":   durationString(timeouts.Init),
	}
}

func renderCircuitBreaker(breaker *config.CircuitBreakerConfig) any {
	if breaker == nil {
		return nil
	}
	return map[string]any{
		"threshold":   breaker.Threshold,
		"reset_after": durationString(breaker.ResetAfter),
	}
}

func renderSchedules(schedules []config.ScheduleConfig) []map[string]any {
	out := make([]map[string]any, 0, len(schedules))
	for _, schedule := range schedules {
		out = append(out, map[string]any{
			"id":               schedule.ID,
			"every":            schedule.Every,
			"cron":             schedule.Cron,
			"at":               schedule.At,
			"after":            durationString(schedule.After),
			"jitter":           durationString(schedule.Jitter),
			"catch_up":         schedule.CatchUp,
			"if_running":       schedule.IfRunning,
			"only_between":     schedule.OnlyBetween,
			"timezone":         schedule.Timezone,
			"not_on":           schedule.NotOn,
			"command":          schedule.Command,
			"payload":          schedule.Payload,
			"preferred_window": schedule.PreferredWindow,
		})
	}
	return out
}

func renderPipelines(pipelines []config.PipelineEntry) []map[string]any {
	out := make([]map[string]any, 0, len(pipelines))
	for _, pipeline := range pipelines {
		out = append(out, map[string]any{
			"name":           pipeline.Name,
			"on":             pipeline.On,
			"steps":          renderSteps(pipeline.Steps),
			"execution_mode": string(pipeline.ExecutionMode),
			"timeout":        durationString(pipeline.Timeout),
		})
	}
	return out
}

func renderSteps(steps []config.StepEntry) []map[string]any {
	out := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		out = append(out, map[string]any{
			"id":    step.ID,
			"uses":  step.Uses,
			"call":  step.Call,
			"steps": renderSteps(step.Steps),
			"split": renderSteps(step.Split),
			"with":  step.With,
		})
	}
	return out
}
