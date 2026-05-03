package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Sender delivers authenticated relay envelopes to named remote instances.
type Sender struct {
	cfg    senderConfig
	now    func() time.Time
	client func(timeout time.Duration) httpDoer
}

// Send posts one relay envelope to a configured remote instance.
func (s *Sender) Send(ctx context.Context, instanceName string, envelope Envelope) (*AcceptanceResponse, error) {
	if s == nil {
		return nil, fmt.Errorf("relay sender is nil")
	}
	instance, ok := s.cfg.Instances[strings.TrimSpace(instanceName)]
	if !ok {
		return nil, fmt.Errorf("relay instance %q not configured", instanceName)
	}
	if len(instance.Allow) > 0 {
		if _, ok := instance.Allow[strings.TrimSpace(envelope.Event.Type)]; !ok {
			return nil, fmt.Errorf("relay instance %q does not allow event type %q", instanceName, envelope.Event.Type)
		}
	}
	if strings.TrimSpace(envelope.Origin.Instance) == "" {
		envelope.Origin.Instance = s.cfg.ServiceName
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal relay envelope: %w", err)
	}

	endpoint := instance.BaseURL + instance.IngressPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build relay request: %w", err)
	}

	timestamp := nowRFC3339(s.now)
	signature, err := sign(http.MethodPost, req.URL.Path, timestamp, body, instance.Secret)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderPeer, s.cfg.ServiceName)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderSignature, signature)
	if instance.KeyID != "" {
		req.Header.Set(HeaderKeyID, instance.KeyID)
	}

	doer := s.client
	if doer == nil {
		doer = func(timeout time.Duration) httpDoer {
			return &http.Client{Timeout: timeout}
		}
	}
	resp, err := doer(instance.Timeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("deliver relay request to %s: %w", instanceName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		var failure ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&failure); err == nil && strings.TrimSpace(failure.Error) != "" {
			return nil, fmt.Errorf("relay receiver rejected request: %s", failure.Error)
		}
		return nil, fmt.Errorf("relay receiver returned status %d", resp.StatusCode)
	}

	var accepted AcceptanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		return nil, fmt.Errorf("decode relay acceptance: %w", err)
	}
	return &accepted, nil
}
