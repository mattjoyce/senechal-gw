package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
)

func runAPINoun(args []string) int {
	if len(args) < 1 || isHelpToken(args[0]) {
		printAPIHelp()
		return 0
	}

	endpoint := args[0]
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	actionArgs := args[1:]

	var method, apiURL, apiKey, bodyStr, configPath string
	var headers, fields stringSlice

	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.StringVar(&method, "method", "", "HTTP method")
	fs.StringVar(&method, "X", "", "HTTP method (alias)")
	fs.StringVar(&apiURL, "api-url", "", "Base API URL")
	fs.StringVar(&apiKey, "api-key", "", "API Key")
	fs.StringVar(&bodyStr, "body", "", "Request body (or - for stdin)")
	fs.StringVar(&bodyStr, "b", "", "Request body (alias)")
	fs.Var(&headers, "header", "HTTP header (Header: value)")
	fs.Var(&headers, "H", "HTTP header (alias)")
	fs.Var(&fields, "field", "JSON field (key=value)")
	fs.Var(&fields, "f", "JSON field (alias)")
	fs.StringVar(&configPath, "config", "", "Path to configuration")
	fs.StringVar(&configPath, "c", "", "Path to configuration (alias)")

	if err := fs.Parse(actionArgs); err != nil {
		return 1
	}

	if method == "" {
		if len(fields) > 0 || bodyStr != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	// Load config to find defaults
	if apiURL == "" || apiKey == "" {
		resolvedPath := configPath
		if resolvedPath == "" {
			// Try to discover config directory
			if dir, err := config.DiscoverConfigDir(); err == nil {
				resolvedPath = dir
			} else {
				resolvedPath = "."
			}
		}
		cfg, _ := config.Load(resolvedPath) // ignore error, might not have config
		if cfg != nil {
			if apiURL == "" && cfg.API.Listen != "" {
				apiURL = cfg.API.Listen
				if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
					apiURL = "http://" + apiURL
				}
			}
			if apiKey == "" && len(cfg.API.Auth.Tokens) > 0 {
				apiKey = cfg.API.Auth.Tokens[0].Token
			}
		}
	}

	// Check env vars as fallback
	if apiKey == "" {
		apiKey = os.Getenv("DUCTILE_API_KEY")
	}
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	var bodyReader io.Reader
	if bodyStr == "-" {
		bodyReader = os.Stdin
	} else if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	} else if len(fields) > 0 {
		fieldMap := make(map[string]any)
		for _, f := range fields {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "Invalid field format %q, expected key=value\n", f)
				return 1
			}
			val := parts[1]
			// Try to parse as JSON types (bool, int)
			if b, err := strconv.ParseBool(val); err == nil {
				fieldMap[parts[0]] = b
			} else if i, err := strconv.Atoi(val); err == nil {
				fieldMap[parts[0]] = i
			} else {
				fieldMap[parts[0]] = val
			}
		}
		data, err := json.Marshal(fieldMap)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal fields: %v\n", err)
			return 1
		}
		bodyReader = bytes.NewReader(data)
	}

	fullURL, err := buildValidatedGatewayAPIURL(apiURL, endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid API URL: %v\n", err)
		return 1
	}
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
		return 1
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid header format %q, expected Header: value\n", h)
			return 1
		}
		req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Request failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		return 1
	}

	if len(body) > 0 {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, body, "", "  "); err == nil {
			fmt.Println(pretty.String())
		} else {
			fmt.Println(string(body))
		}
	}

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Response Status: %s\n", resp.Status)
		return 1
	}

	return 0
}

func printAPIHelp() {
	fmt.Print(`Usage: ductile api <endpoint> [flags]

Directly call the gateway API using your current configuration for URL and token.

Arguments:
  endpoint      The API path (e.g., /jobs, /plugin/echo)

Flags:
  -X, --method  HTTP method (default: GET, or POST if fields/body provided)
  -f, --field   Add a JSON field (key=value). May be used multiple times.
  -H, --header  Add an HTTP header (Header: value). May be used multiple times.
  -b, --body    Raw request body (or - for stdin)
  -c, --config  Path to configuration to load defaults from
  --api-url     Override base API URL
  --api-key     Override API key (defaults to first token in config or DUCTILE_API_KEY)

Examples:
  ductile api /jobs
  ductile api /plugin/echo/poll -f message="hello"
  ductile api /system/reload -X POST
`)
}

func buildValidatedGatewayAPIURL(base, endpoint string) (string, error) {
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("base URL is required")
	}
	if endpoint == "" || !strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "//") {
		return "", fmt.Errorf("endpoint must be an absolute path")
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if baseURL.Hostname() == "" {
		return "", fmt.Errorf("host is required")
	}
	if err := validateGatewayAPIHost(baseURL.Hostname()); err != nil {
		return "", err
	}

	relative, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	if relative.IsAbs() || relative.Host != "" {
		return "", fmt.Errorf("endpoint must not include a host")
	}

	baseURL.Path = path.Join(baseURL.Path, relative.Path)
	if strings.HasSuffix(relative.Path, "/") && !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}
	baseURL.RawQuery = relative.RawQuery
	baseURL.Fragment = ""
	return baseURL.String(), nil
}

func validateGatewayAPIHost(host string) error {
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host must be localhost or an IP address")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return nil
	}
	return fmt.Errorf("host must be loopback, private, or link-local")
}
