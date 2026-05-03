package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	relayInstanceNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	relayEventTypePattern    = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
)

func validateRelayConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	relayConfigured := len(cfg.RelayInstances) > 0 || cfg.RemoteIngress != nil
	if relayConfigured {
		serviceName := strings.TrimSpace(cfg.Service.Name)
		if serviceName == "" {
			return fmt.Errorf("service.name is required when relay is configured")
		}
		if !relayInstanceNamePattern.MatchString(serviceName) {
			return fmt.Errorf("service.name %q must use lower-case hyphenated form", cfg.Service.Name)
		}
	}

	instanceNames := make(map[string]struct{}, len(cfg.RelayInstances))
	for i, instance := range cfg.RelayInstances {
		name := strings.TrimSpace(instance.Name)
		if name == "" {
			return fmt.Errorf("instances[%d].name is required", i)
		}
		if !relayInstanceNamePattern.MatchString(name) {
			return fmt.Errorf("instances[%d].name %q must use lower-case hyphenated form", i, instance.Name)
		}
		if _, exists := instanceNames[name]; exists {
			return fmt.Errorf("duplicate relay instance name %q", name)
		}
		instanceNames[name] = struct{}{}

		if strings.TrimSpace(instance.BaseURL) == "" {
			return fmt.Errorf("instances[%d] (%s): base_url is required", i, name)
		}
		parsedURL, err := url.Parse(strings.TrimSpace(instance.BaseURL))
		if err != nil || parsedURL == nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return fmt.Errorf("instances[%d] (%s): base_url %q must be an absolute http or https URL", i, name, instance.BaseURL)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("instances[%d] (%s): base_url scheme must be http or https", i, name)
		}

		if err := validateListenPath(instance.IngressPath); err != nil {
			return fmt.Errorf("instances[%d] (%s): invalid ingress_path: %w", i, name, err)
		}
		if strings.TrimSpace(instance.SecretRef) == "" {
			return fmt.Errorf("instances[%d] (%s): secret_ref is required", i, name)
		}
		if instance.Timeout < 0 {
			return fmt.Errorf("instances[%d] (%s): timeout must be >= 0", i, name)
		}
		if err := validateEventTypeList(fmt.Sprintf("instances[%d] (%s).allow", i, name), instance.Allow); err != nil {
			return err
		}
	}

	if cfg.RemoteIngress == nil {
		return nil
	}

	if err := validateListenPath(cfg.RemoteIngress.ListenPath); err != nil {
		return fmt.Errorf("remote_ingress.listen_path: %w", err)
	}
	if strings.TrimSpace(cfg.RemoteIngress.MaxBodySize) != "" {
		if _, err := ParseByteSize(cfg.RemoteIngress.MaxBodySize); err != nil {
			return fmt.Errorf("remote_ingress.max_body_size: %w", err)
		}
	}
	if cfg.RemoteIngress.AllowedClockSkew < 0 {
		return fmt.Errorf("remote_ingress.allowed_clock_skew must be >= 0")
	}

	peerNames := make(map[string]struct{}, len(cfg.RemoteIngress.TrustedPeers))
	for i, peer := range cfg.RemoteIngress.TrustedPeers {
		name := strings.TrimSpace(peer.Name)
		if name == "" {
			return fmt.Errorf("remote_ingress.peers[%d].name is required", i)
		}
		if !relayInstanceNamePattern.MatchString(name) {
			return fmt.Errorf("remote_ingress.peers[%d].name %q must use lower-case hyphenated form", i, peer.Name)
		}
		if _, exists := peerNames[name]; exists {
			return fmt.Errorf("duplicate relay peer name %q", name)
		}
		peerNames[name] = struct{}{}

		if strings.TrimSpace(peer.SecretRef) == "" {
			return fmt.Errorf("remote_ingress.peers[%d] (%s): secret_ref is required", i, name)
		}
		if cfg.RemoteIngress.RequireKeyID && strings.TrimSpace(peer.KeyID) == "" {
			return fmt.Errorf("remote_ingress.peers[%d] (%s): key_id is required when remote_ingress.require_key_id is true", i, name)
		}
		if err := validateEventTypeList(fmt.Sprintf("remote_ingress.peers[%d] (%s).accept", i, name), peer.Accept); err != nil {
			return err
		}
		for j, baggageKey := range peer.Baggage.Allow {
			if strings.TrimSpace(baggageKey) == "" {
				return fmt.Errorf("remote_ingress.peers[%d] (%s).baggage.allow[%d] must be non-empty", i, name, j)
			}
		}
	}

	return nil
}

func validateListenPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /")
	}
	if strings.Contains(path, " ") {
		return fmt.Errorf("path must not contain spaces")
	}
	return nil
}

func validateEventTypeList(path string, events []string) error {
	seen := make(map[string]struct{}, len(events))
	for i, eventType := range events {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			return fmt.Errorf("%s[%d] must be non-empty", path, i)
		}
		if !relayEventTypePattern.MatchString(eventType) {
			return fmt.Errorf("%s[%d] %q must use lower-case dotted form", path, i, events[i])
		}
		if _, exists := seen[eventType]; exists {
			return fmt.Errorf("%s contains duplicate event type %q", path, eventType)
		}
		seen[eventType] = struct{}{}
	}
	return nil
}

func defaultRelayRequestTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 10 * time.Second
}

func defaultRelayAllowedClockSkew(skew time.Duration) time.Duration {
	if skew > 0 {
		return skew
	}
	return 5 * time.Minute
}
