package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattjoyce/ductile/internal/relay"
)

func runRelayNoun(args []string) int {
	if len(args) < 1 {
		printRelayNounHelp(os.Stderr)
		return 1
	}

	if isHelpToken(args[0]) {
		printRelayNounHelp(os.Stdout)
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "send":
		if hasHelpFlag(actionArgs) {
			printRelaySendHelp()
			return 0
		}
		return runRelaySend(actionArgs)
	case "help":
		printRelayNounHelp(os.Stdout)
		return 0
	default:
		// #nosec G705 -- stderr output is plain text, not HTML.
		fmt.Fprintf(os.Stderr, "Unknown relay action: %s\n", action)
		return 1
	}
}

func printRelayNounHelp(w *os.File) {
	_, _ = fmt.Fprintln(w, "Usage: ductile relay <action>")
	_, _ = fmt.Fprintln(w, "Actions: send")
}

func printRelaySendHelp() {
	fmt.Println("Usage: ductile relay send <instance> --event TYPE [--payload JSON|--payload-file PATH] [--baggage JSON|--baggage-file PATH] [--dedupe-key KEY] [--origin-plugin NAME] [--origin-job-id ID] [--origin-event-id ID] [--wait [--timeout DUR]] [--config PATH | --config-dir PATH] [--json]")
	fmt.Println("Send one authenticated relay event to a configured remote instance.")
	fmt.Println("With --wait the receiver blocks on the triggered pipeline tree and returns its result (requires receiver-side sync policy + peer allow_sync).")
}

func runRelaySend(args []string) int {
	fs := flag.NewFlagSet("relay send", flag.ContinueOnError)
	eventType := fs.String("event", "", "Event type to relay")
	payloadRaw := fs.String("payload", "", "JSON payload object")
	payloadFile := fs.String("payload-file", "", "Path to JSON payload object file")
	baggageRaw := fs.String("baggage", "", "JSON baggage object")
	baggageFile := fs.String("baggage-file", "", "Path to JSON baggage object file")
	dedupeKey := fs.String("dedupe-key", "", "Optional event dedupe key")
	originPlugin := fs.String("origin-plugin", "", "Optional origin plugin name")
	originJobID := fs.String("origin-job-id", "", "Optional origin job id")
	originEventID := fs.String("origin-event-id", "", "Optional origin event id")
	configPath := fs.String("config", "", "Path to configuration file")
	configDir := fs.String("config-dir", "", "Path to configuration directory")
	wait := fs.Bool("wait", false, "Request a synchronous reply: block until the receiver's pipeline tree settles")
	waitTimeout := fs.String("timeout", "", "Requested wait budget for --wait (e.g. 30s); receiver clamps to its own maximum")
	jsonOut := fs.Bool("json", false, "Output acceptance JSON")
	// Support both <instance>-first (per usage string) and flags-first orderings
	// by lifting a leading positional out before flag.Parse, since the standard
	// flag package stops at the first non-flag token.
	var leadingInstance string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		leadingInstance = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	var instanceName string
	switch {
	case leadingInstance != "" && fs.NArg() == 0:
		instanceName = strings.TrimSpace(leadingInstance)
	case leadingInstance == "" && fs.NArg() == 1:
		instanceName = strings.TrimSpace(fs.Arg(0))
	default:
		printRelaySendHelp()
		return 1
	}
	if instanceName == "" {
		fmt.Fprintln(os.Stderr, "Error: relay instance name is required")
		return 1
	}
	if strings.TrimSpace(*eventType) == "" {
		fmt.Fprintln(os.Stderr, "Error: --event is required")
		return 1
	}

	payload, err := loadJSONObjectInput(*payloadRaw, *payloadFile, "payload")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if payload == nil {
		payload = map[string]any{}
	}

	baggageValues, err := loadJSONObjectInput(*baggageRaw, *baggageFile, "baggage")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	cfg, err := loadConfigForToolWithDir(*configPath, *configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		return 1
	}

	sender, err := relay.NewSender(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Relay sender error: %v\n", err)
		return 1
	}

	var reply *relay.EnvelopeReply
	if *wait {
		reply = &relay.EnvelopeReply{
			Mode:    relay.SyncReplyMode,
			Timeout: strings.TrimSpace(*waitTimeout),
		}
	}

	accepted, err := sender.Send(context.Background(), instanceName, relay.Envelope{
		Event: relay.EnvelopeEvent{
			Type:      strings.TrimSpace(*eventType),
			Payload:   payload,
			DedupeKey: strings.TrimSpace(*dedupeKey),
		},
		Origin: relay.EnvelopeOrigin{
			Plugin:  strings.TrimSpace(*originPlugin),
			JobID:   strings.TrimSpace(*originJobID),
			EventID: strings.TrimSpace(*originEventID),
		},
		Baggage: baggageValues,
		Reply:   reply,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Relay send failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		data, err := json.MarshalIndent(accepted, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render response: %v\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	fmt.Printf("Relayed %s to %s\n", strings.TrimSpace(*eventType), instanceName)
	fmt.Printf("Receiver event id: %s\n", accepted.ReceiverEventID)
	if strings.TrimSpace(accepted.JobID) != "" {
		fmt.Printf("Receiver job id: %s\n", accepted.JobID)
	}
	if *wait {
		if accepted.TimedOut {
			fmt.Printf("Status: %s (wait timed out after %dms — outcome unknown; safe to retry only if dedupe-keyed)\n", accepted.Status, accepted.DurationMs)
			return 2
		}
		fmt.Printf("Status: %s (%dms)\n", accepted.Status, accepted.DurationMs)
		if len(accepted.Result) > 0 {
			fmt.Printf("Result: %s\n", string(accepted.Result))
		}
		if accepted.Status != "succeeded" {
			return 1
		}
	}
	return 0
}

func loadJSONObjectInput(rawValue, filePath, fieldName string) (map[string]any, error) {
	if strings.TrimSpace(rawValue) != "" && strings.TrimSpace(filePath) != "" {
		return nil, fmt.Errorf("use only one of --%s or --%s-file", fieldName, fieldName)
	}

	var data []byte
	switch {
	case strings.TrimSpace(filePath) != "":
		readData, err := readFileWithinParent(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s file: %w", fieldName, err)
		}
		data = bytes.TrimSpace(readData)
	case strings.TrimSpace(rawValue) != "":
		data = bytes.TrimSpace([]byte(rawValue))
	default:
		return nil, nil
	}

	if len(data) == 0 {
		return map[string]any{}, nil
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("%s must be valid JSON", fieldName)
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", fieldName, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}

	obj, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", fieldName)
	}
	return obj, nil
}

func readFileWithinParent(path string) ([]byte, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(absPath))
	if err != nil {
		return nil, fmt.Errorf("open parent root: %w", err)
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(filepath.Base(absPath))
}
