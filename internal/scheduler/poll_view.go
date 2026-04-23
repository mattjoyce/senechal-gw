package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattjoyce/ductile/internal/events"
	"github.com/mattjoyce/ductile/internal/queue"
)

type pollJob struct {
	Plugin     string
	Command    string
	ScheduleID string
	Status     string
}

func (s *Scheduler) bootstrapPollView(ctx context.Context) error {
	polls := make(map[string]pollJob)
	for _, status := range []queue.Status{queue.StatusQueued, queue.StatusRunning} {
		jobs, err := s.queue.FindJobsByStatus(ctx, status)
		if err != nil {
			return fmt.Errorf("find %s jobs: %w", status, err)
		}
		for _, job := range jobs {
			if !s.isSchedulerPollJob(job.Plugin, job.Command, job.SubmittedBy) {
				continue
			}
			polls[job.ID] = pollJob{
				Plugin:     job.Plugin,
				Command:    job.Command,
				ScheduleID: scheduleIDFromDedupeKey(job.Plugin, job.Command, job.DedupeKey),
				Status:     string(status),
			}
		}
	}

	s.pollsMu.Lock()
	s.polls = polls
	s.pollsMu.Unlock()
	return nil
}

func (s *Scheduler) startPollEventConsumer(ctx context.Context) {
	ch, cancel := s.events.Subscribe()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				s.applyPollEvent(ev)
			}
		}
	}()
}

func (s *Scheduler) applyPollEvent(ev events.Event) {
	switch ev.Type {
	case "scheduler.scheduled", "poll.started":
	case "poll.completed":
	default:
		return
	}

	var data map[string]any
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		return
	}

	jobID := stringFromEventData(data, "job_id")
	if strings.TrimSpace(jobID) == "" {
		return
	}

	if ev.Type == "poll.completed" {
		s.pollsMu.Lock()
		delete(s.polls, jobID)
		s.pollsMu.Unlock()
		return
	}

	plugin := stringFromEventData(data, "plugin")
	command := stringFromEventData(data, "command")
	if command == "" {
		command = pollCommand
	}
	submittedBy := stringFromEventData(data, "submitted_by")
	if submittedBy == "" {
		submittedBy = s.schedulerSubmitter()
	}
	if !s.isSchedulerPollJob(plugin, command, submittedBy) {
		return
	}

	status := string(queue.StatusQueued)
	if ev.Type == "poll.started" {
		status = string(queue.StatusRunning)
	}

	s.pollsMu.Lock()
	if s.polls == nil {
		s.polls = make(map[string]pollJob)
	}
	s.polls[jobID] = pollJob{
		Plugin:     plugin,
		Command:    command,
		ScheduleID: stringFromEventData(data, "schedule_id"),
		Status:     status,
	}
	s.pollsMu.Unlock()
}

func (s *Scheduler) pollOutstanding(ctx context.Context, pluginName, command string) (int, error) {
	viewCount := s.pollViewCount(pluginName, command)
	if viewCount > 0 {
		return viewCount, nil
	}

	// Temporary safety net while the event-derived view proves itself: if this
	// process missed an event, queue truth prevents duplicate scheduler polls.
	queueCount, err := s.queue.CountOutstandingJobs(ctx, pluginName, command)
	if err != nil {
		return 0, err
	}
	return queueCount, nil
}

func (s *Scheduler) pollViewCount(pluginName, command string) int {
	s.pollsMu.Lock()
	defer s.pollsMu.Unlock()

	count := 0
	for _, poll := range s.polls {
		if poll.Plugin == pluginName && poll.Command == command {
			count++
		}
	}
	return count
}

func (s *Scheduler) clearPollsFor(pluginName, command string) {
	s.pollsMu.Lock()
	defer s.pollsMu.Unlock()

	for jobID, poll := range s.polls {
		if poll.Plugin == pluginName && poll.Command == command {
			delete(s.polls, jobID)
		}
	}
}

func (s *Scheduler) isSchedulerPollJob(pluginName, command, submittedBy string) bool {
	return strings.TrimSpace(pluginName) != "" &&
		strings.TrimSpace(command) == pollCommand &&
		strings.TrimSpace(submittedBy) == s.schedulerSubmitter()
}

func (s *Scheduler) schedulerSubmitter() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.Service.Name)
}

func stringFromEventData(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func scheduleIDFromDedupeKey(pluginName, command string, dedupeKey *string) string {
	if dedupeKey == nil {
		return ""
	}
	raw := strings.TrimSpace(*dedupeKey)
	prefix := pluginName + ":" + command + ":"
	if raw == "" || !strings.HasPrefix(raw, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(raw, prefix)
	if rest == "" {
		return ""
	}
	parts := strings.Split(rest, ":")
	return strings.TrimSpace(parts[0])
}
