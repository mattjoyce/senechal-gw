package watch

import (
	"time"
)

// HealthState tracks gateway health from /healthz polling.
type HealthState struct {
	Status        string
	UptimeSeconds int64
	QueueDepth    int
	PluginsLoaded int
	ConfigPath    string
	BinaryPath    string
	Version       string
	Connected     bool
	LastCheck     time.Time
}
