package relay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRelayMaxBodySize       = 1024 * 1024
	defaultRelayRequestTimeout    = 10 * time.Second
	defaultRelayAllowedClockSkew  = 5 * time.Minute
	defaultRelaySyncMaxTimeout    = 60 * time.Second
	defaultRelaySyncMaxConcurrent = 4
)

// syncRequested reports whether the signed envelope asks for a synchronous
// reply. Mode comparison is case-insensitive; anything other than "sync"
// (including absent) means fire-and-forget.
func syncRequested(env Envelope) bool {
	return env.Reply != nil && strings.EqualFold(strings.TrimSpace(env.Reply.Mode), SyncReplyMode)
}

// clampSyncTimeout resolves the wait budget. The receiver is authoritative:
// the caller may ask for less than the receiver's maximum but never more, and
// an absent or unparseable request falls back to the receiver's maximum.
func clampSyncTimeout(reply *EnvelopeReply, max time.Duration) time.Duration {
	if max <= 0 {
		max = defaultRelaySyncMaxTimeout
	}
	if reply == nil {
		return max
	}
	raw := strings.TrimSpace(reply.Timeout)
	if raw == "" {
		return max
	}
	requested, err := time.ParseDuration(raw)
	if err != nil || requested <= 0 || requested > max {
		return max
	}
	return requested
}

func normalizeSyncMaxTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defaultRelaySyncMaxTimeout
}

func normalizeSyncMaxConcurrent(n int) int {
	if n > 0 {
		return n
	}
	return defaultRelaySyncMaxConcurrent
}

var (
	eventTypePattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

func validateEnvelope(raw []byte) (*Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid JSON body")
	}

	eventType := strings.TrimSpace(envelope.Event.Type)
	if eventType == "" {
		return nil, fmt.Errorf("event.type is required")
	}
	if !eventTypePattern.MatchString(eventType) {
		return nil, fmt.Errorf("event.type must use lower-case dotted form")
	}
	envelope.Event.Type = eventType

	if envelope.Event.Payload == nil {
		envelope.Event.Payload = map[string]any{}
	}
	if envelope.Baggage == nil {
		envelope.Baggage = map[string]any{}
	}
	return &envelope, nil
}

func parseRelayTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("timestamp is required")
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts.UTC(), nil
	}
	if unixSeconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(unixSeconds, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("timestamp must be RFC3339 or unix seconds")
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func nowRFC3339(clock func() time.Time) string {
	if clock == nil {
		return nowUTC().Format(time.RFC3339Nano)
	}
	return clock().UTC().Format(time.RFC3339Nano)
}

func normalizeListenPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/ingest/peer"
	}
	if path == "/" {
		return path
	}
	return strings.TrimRight(path, "/")
}

func normalizeRequestTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return defaultRelayRequestTimeout
}

func normalizeAllowedClockSkew(skew time.Duration) time.Duration {
	if skew > 0 {
		return skew
	}
	return defaultRelayAllowedClockSkew
}
