package msgs

import (
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// --- Tick messages ---

type LiveTickMsg struct{}
type FutureTickMsg struct{}
type PastTickMsg struct{}

// --- Data loaded messages ---

type HealthLoadedMsg struct {
	Data types.RuntimeHealth
	Err  error
}

type EventsLoadedMsg struct {
	Data []types.Event
	Err  error
}

type JobsLoadedMsg struct {
	Data  []types.Job
	Total int
	Err   error
}

type JobLogsLoadedMsg struct {
	Data  []types.JobLog
	Total int
	Err   error
}

type JobDetailLoadedMsg struct {
	Data types.JobDetail
	Err  error
}

type SchedulerLoadedMsg struct {
	Data []types.SchedulerJob
	Err  error
}

type PluginsLoadedMsg struct {
	Data []types.PluginSummary
	Err  error
}

// --- SSE messages ---

type SSEEventMsg struct {
	Event types.Event
}

type SSEDisconnectedMsg struct{}
type SSEReconnectMsg struct{}

// --- Navigation ---

type OpenDetailMsg struct {
	Target types.DetailTarget
}

type NavigateBackMsg struct{}

// --- UI ---

type FilterChangedMsg struct{ Query string }
type FreezeToggleMsg struct{}
type ForceRefreshMsg struct{}
