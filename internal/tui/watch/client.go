package watch

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattjoyce/ductile/internal/events"
)

// --- Message types ---

type eventMsg events.Event

type healthMsg struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	QueueDepth    int    `json:"queue_depth"`
	PluginsLoaded int    `json:"plugins_loaded"`
}

type tickMsg time.Time

type errMsg error

type sseDisconnectedMsg struct{}
type reconnectMsg struct{}

// --- Commands ---

// subscribeToEvents connects to the SSE /events endpoint and feeds events
// into the provided channel. Returns sseDisconnectedMsg when the connection drops.
func subscribeToEvents(apiURL, apiKey string, ch chan<- events.Event) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{}
		req, err := http.NewRequest("GET", apiURL+"/events", nil)
		if err != nil {
			return errMsg(err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return sseDisconnectedMsg{}
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var current struct {
			id   int64
			typ  string
			data string
		}

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				if current.data != "" {
					ch <- events.Event{
						ID:   current.id,
						Type: current.typ,
						At:   time.Now(),
						Data: []byte(current.data),
					}
					current = struct {
						id   int64
						typ  string
						data string
					}{}
				}
				continue
			}

			if strings.HasPrefix(line, "id: ") {
				if id, err := strconv.ParseInt(line[4:], 10, 64); err == nil {
					current.id = id
				}
			} else if strings.HasPrefix(line, "event: ") {
				current.typ = line[7:]
			} else if strings.HasPrefix(line, "data: ") {
				current.data = line[6:]
			}
		}

		return sseDisconnectedMsg{}
	}
}

// receiveNextEvent waits for the next event from the channel.
func receiveNextEvent(ch <-chan events.Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-ch)
	}
}

// fetchHealth queries the /healthz endpoint.
func fetchHealth(apiURL, apiKey string) tea.Msg {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest("GET", apiURL+"/healthz", nil)
	if err != nil {
		return errMsg(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return errMsg(err)
	}
	defer resp.Body.Close()

	var h healthMsg
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return errMsg(err)
	}
	return h
}
