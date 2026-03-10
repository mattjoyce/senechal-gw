package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mattjoyce/ductile/internal/tui/types"
)

// Client talks to the live ductile API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a client for the given ductile API.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) BaseURL() string { return c.baseURL }
func (c *Client) APIKey() string  { return c.apiKey }

// Health fetches GET /healthz.
func (c *Client) Health(ctx context.Context) (types.RuntimeHealth, error) {
	var h types.RuntimeHealth
	err := c.getJSON(ctx, "/healthz", nil, &h)
	return h, err
}

// ListJobs fetches GET /jobs with optional filters.
func (c *Client) ListJobs(ctx context.Context, plugin, command, status string, limit int) ([]types.Job, int, error) {
	params := url.Values{}
	if plugin != "" {
		params.Set("plugin", plugin)
	}
	if command != "" {
		params.Set("command", command)
	}
	if status != "" {
		params.Set("status", status)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	var resp struct {
		Jobs  []types.Job `json:"jobs"`
		Total int         `json:"total"`
	}
	err := c.getJSON(ctx, "/jobs", params, &resp)
	return resp.Jobs, resp.Total, err
}

// GetJob fetches GET /job/{id}.
func (c *Client) GetJob(ctx context.Context, jobID string) (types.JobDetail, error) {
	var d types.JobDetail
	err := c.getJSON(ctx, "/job/"+jobID, nil, &d)
	return d, err
}

// ListJobLogs fetches GET /job-logs with optional filters.
func (c *Client) ListJobLogs(ctx context.Context, plugin, status string, since, until *time.Time, limit int) ([]types.JobLog, int, error) {
	params := url.Values{}
	if plugin != "" {
		params.Set("plugin", plugin)
	}
	if status != "" {
		params.Set("status", status)
	}
	if since != nil {
		params.Set("from", since.Format(time.RFC3339))
	}
	if until != nil {
		params.Set("to", until.Format(time.RFC3339))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	var resp struct {
		Logs  []types.JobLog `json:"logs"`
		Total int            `json:"total"`
	}
	err := c.getJSON(ctx, "/job-logs", params, &resp)
	return resp.Logs, resp.Total, err
}

// SchedulerJobs fetches GET /scheduler/jobs.
func (c *Client) SchedulerJobs(ctx context.Context) ([]types.SchedulerJob, error) {
	var resp struct {
		Jobs []types.SchedulerJob `json:"jobs"`
	}
	err := c.getJSON(ctx, "/scheduler/jobs", nil, &resp)
	return resp.Jobs, err
}

// ListPlugins fetches GET /plugins.
func (c *Client) ListPlugins(ctx context.Context) ([]types.PluginSummary, error) {
	var resp struct {
		Plugins []types.PluginSummary `json:"plugins"`
	}
	err := c.getJSON(ctx, "/plugins", nil, &resp)
	return resp.Plugins, err
}

// GetPlugin fetches GET /plugin/{name}.
func (c *Client) GetPlugin(ctx context.Context, name string) (types.PluginDetail, error) {
	var d types.PluginDetail
	err := c.getJSON(ctx, "/plugin/"+name, nil, &d)
	return d, err
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, dest any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request %s: status %d", path, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
