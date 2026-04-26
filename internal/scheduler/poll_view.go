package scheduler

import (
	"context"
	"strings"
)

// outstandingForPolicy returns the number of currently-active jobs that
// constitute the scheduler's parallelism budget for (plugin, command).
//
// For poll commands the count is restricted to jobs the scheduler itself
// submitted, so externally-submitted polls (CLI/webhook/router) do not
// consume the scheduler's budget. For other commands the unfiltered count
// is used, since dedupe semantics there apply across all submitters.
//
// The job_queue is the single authoritative substrate; the scheduler used
// to maintain a parallel event-derived view, removed in Sprint 15 of the
// Hickey refactor.
func (s *Scheduler) outstandingForPolicy(ctx context.Context, pluginName, command string) (int, error) {
	if command == pollCommand {
		return s.queue.CountOutstandingJobsBySubmitter(ctx, pluginName, command, s.schedulerSubmitter())
	}
	return s.queue.CountOutstandingJobs(ctx, pluginName, command)
}

func (s *Scheduler) schedulerSubmitter() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.Service.Name)
}
