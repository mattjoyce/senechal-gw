package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/doctor"
	"github.com/mattjoyce/ductile/internal/plugin"
	"github.com/mattjoyce/ductile/internal/tui/tokenmgr"
	"gopkg.in/yaml.v3"
)

type scopeDoc struct {
	Scopes   []string       `json:"scopes"`
	Metadata scopeDocFields `json:"metadata"`
}

type scopeDocFields struct {
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type tokenCreateJSONOutput struct {
	Status     string            `json:"status"`
	Token      config.TokenEntry `json:"token"`
	TokenKey   string            `json:"token_key"`
	EnvVar     string            `json:"env_var"`
	Validation *doctor.Result    `json:"validation,omitempty"`
}

type tokenInspectJSONOutput struct {
	Name         string   `json:"name"`
	Key          string   `json:"key"`
	ScopesFile   string   `json:"scopes_file,omitempty"`
	ScopesHash   string   `json:"scopes_hash,omitempty"`
	CurrentHash  string   `json:"current_hash,omitempty"`
	HashMatches  bool     `json:"hash_matches"`
	Scopes       []string `json:"scopes,omitempty"`
	Description  string   `json:"description,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
	ConfigDir    string   `json:"config_dir"`
	ConfigTarget string   `json:"config_target"`
}

func runConfigToken(args []string) int {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigTokenHelp()
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "create":
		return runConfigTokenCreate(actionArgs)
	case "list":
		return runConfigTokenList(actionArgs)
	case "inspect":
		return runConfigTokenInspect(actionArgs)
	case "rehash":
		return runConfigTokenRehash(actionArgs)
	case "delete":
		return runConfigTokenDelete(actionArgs)
	case "help":
		printConfigTokenHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config token action: %s\n", action)
		return 1
	}
}

func runConfigScope(args []string) int {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigScopeHelp()
		return 0
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "add":
		return runConfigScopeAdd(actionArgs)
	case "remove":
		return runConfigScopeRemove(actionArgs)
	case "set":
		return runConfigScopeSet(actionArgs)
	case "validate":
		return runConfigScopeValidate(actionArgs)
	case "help":
		printConfigScopeHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config scope action: %s\n", action)
		return 1
	}
}

func runConfigTokenCreate(args []string) int {
	var configPath, configDir, name, scopesArg, scopesFile, description, format string
	var useTUI bool

	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&name, "name", "", "Token name")
	fs.StringVar(&scopesArg, "scopes", "", "Comma-separated scopes")
	fs.StringVar(&scopesFile, "scopes-file", "", "Path to scopes JSON file (or - for stdin)")
	fs.StringVar(&description, "description", "", "Token description")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	fs.BoolVar(&useTUI, "tui", false, "Use interactive TUI for scope selection")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		return 1
	}

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	var scopes []string
	if useTUI || (scopesArg == "" && scopesFile == "") {
		// Discover plugins for TUI
		cfg, err := config.Load(resolvedPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Load config failed: %v\n", err)
			return 1
		}
		registry, _ := plugin.Discover(cfg.PluginsDir, nil)

		tm := tokenmgr.New(registry)
		p := tea.NewProgram(tm)
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			return 1
		}
		scopes = tm.GetSelectedScopes()
		if len(scopes) == 0 {
			fmt.Fprintln(os.Stderr, "No scopes selected, aborting.")
			return 1
		}
	} else {
		if scopesArg != "" && scopesFile != "" {
			fmt.Fprintln(os.Stderr, "Error: use only one of --scopes or --scopes-file")
			return 1
		}
		scopes, err = parseScopesInput(scopesArg, scopesFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Parse scopes failed: %v\n", err)
			return 1
		}
	}

	if len(scopes) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no scopes provided")
		return 1
	}

	tokensPath := filepath.Join(resolvedDir, "tokens.yaml")
	scopePath := filepath.Join(resolvedDir, "scopes", name+".json")
	if err := os.MkdirAll(filepath.Dir(scopePath), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create scopes dir: %v\n", err)
		return 1
	}

	tokensCfg, err := loadTokensFile(tokensPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}
	if _, idx := findTokenByName(tokensCfg.Tokens, name); idx >= 0 {
		fmt.Fprintf(os.Stderr, "Token %q already exists\n", name)
		return 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	doc := scopeDoc{
		Scopes: scopes,
		Metadata: scopeDocFields{
			Description: description,
			CreatedAt:   now,
		},
	}

	scopeRaw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode scope document: %v\n", err)
		return 1
	}
	scopeRaw = append(scopeRaw, '\n')
	if err := writeFileAtomicWithBackup(scopePath, scopeRaw, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write scope file: %v\n", err)
		return 1
	}

	scopeHash, err := config.ComputeBlake3Hash(scopePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash scope file: %v\n", err)
		return 1
	}

	tokenKey, err := generateSecureToken(32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate token: %v\n", err)
		return 1
	}
	envVar := tokenEnvVarName(name)

	entry := config.TokenEntry{
		Name:        name,
		Key:         fmt.Sprintf("${%s}", envVar),
		ScopesFile:  filepath.ToSlash(filepath.Join("scopes", name+".json")),
		ScopesHash:  "blake3:" + scopeHash,
		Description: description,
		CreatedAt:   now,
	}
	tokensCfg.Tokens = append(tokensCfg.Tokens, entry)

	if err := writeTokensFile(tokensPath, tokensCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write tokens file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}

	if format == "json" {
		out := tokenCreateJSONOutput{
			Status:     "success",
			Token:      entry,
			TokenKey:   tokenKey,
			EnvVar:     envVar,
			Validation: validation,
		}
		encoded, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(encoded))
		return code
	}

	fmt.Printf("Created: %s\n", scopePath)
	fmt.Printf("Updated: %s\n", tokensPath)
	fmt.Printf("Token key: %s\n\n", tokenKey)
	fmt.Printf("Set environment variable:\n  export %s=\"%s\"\n", envVar, tokenKey)
	printValidationSummary(validation)
	return code
}

func runConfigTokenList(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("token list", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	_, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	tokensPath := filepath.Join(resolvedDir, "tokens.yaml")
	tokensCfg, err := loadTokensFile(tokensPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}

	if format == "json" {
		out, _ := json.MarshalIndent(tokensCfg.Tokens, "", "  ")
		fmt.Println(string(out))
		return 0
	}

	fmt.Printf("Tokens in %s:\n", tokensPath)
	if len(tokensCfg.Tokens) == 0 {
		fmt.Println("  (none)")
		return 0
	}

	for _, token := range tokensCfg.Tokens {
		fmt.Printf("\n%s\n", token.Name)
		if token.CreatedAt != "" {
			fmt.Printf("  Created: %s\n", token.CreatedAt)
		}
		if token.ScopesFile != "" {
			scopes, _ := loadTokenScopes(filepath.Join(resolvedDir, filepath.FromSlash(token.ScopesFile)))
			if len(scopes) > 0 {
				fmt.Printf("  Scopes: %s\n", strings.Join(scopes, ", "))
			}
			fmt.Printf("  Scopes file: %s\n", token.ScopesFile)
		}
		if token.Description != "" {
			fmt.Printf("  Description: %s\n", token.Description)
		}
	}
	return 0
}

func runConfigTokenInspect(args []string) int {
	var configPath, configDir, format string

	fs := flag.NewFlagSet("token inspect", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config token inspect <name> [--config PATH] [--config-dir PATH] [--format human|json]")
		return 1
	}
	tokenName := positionals[0]

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	tokensCfg, err := loadTokensFile(filepath.Join(resolvedDir, "tokens.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}
	entry, idx := findTokenByName(tokensCfg.Tokens, tokenName)
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "Token %q not found\n", tokenName)
		return 1
	}

	var scopes []string
	var currentHash string
	hashMatches := false
	scopeFilePath := ""
	if entry.ScopesFile != "" {
		scopeFilePath = filepath.Join(resolvedDir, filepath.FromSlash(entry.ScopesFile))
		scopes, _ = loadTokenScopes(scopeFilePath)
		actualHash, hashErr := config.ComputeBlake3Hash(scopeFilePath)
		if hashErr == nil {
			currentHash = "blake3:" + actualHash
			hashMatches = currentHash == entry.ScopesHash
		}
	}

	if format == "json" {
		out := tokenInspectJSONOutput{
			Name:         entry.Name,
			Key:          entry.Key,
			ScopesFile:   entry.ScopesFile,
			ScopesHash:   entry.ScopesHash,
			CurrentHash:  currentHash,
			HashMatches:  hashMatches,
			Scopes:       scopes,
			Description:  entry.Description,
			CreatedAt:    entry.CreatedAt,
			ConfigDir:    resolvedDir,
			ConfigTarget: resolvedPath,
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
		return 0
	}

	fmt.Printf("Token: %s\n", entry.Name)
	if entry.CreatedAt != "" {
		fmt.Printf("Created: %s\n", entry.CreatedAt)
	}
	fmt.Printf("Key: %s\n", entry.Key)
	if entry.ScopesFile != "" {
		fmt.Printf("Scope file: %s\n", entry.ScopesFile)
	}
	if entry.ScopesHash != "" {
		fmt.Printf("Hash: %s\n", entry.ScopesHash)
	}
	if currentHash != "" {
		fmt.Printf("Current hash: %s\n", currentHash)
	}
	if scopeFilePath != "" {
		status := "mismatch"
		if hashMatches {
			status = "match"
		}
		fmt.Printf("Hash check: %s\n", status)
	}
	if len(scopes) > 0 {
		fmt.Println("Scopes:")
		for _, scope := range scopes {
			fmt.Printf("  - %s\n", scope)
		}
	}
	if entry.Description != "" {
		fmt.Printf("Description: %s\n", entry.Description)
	}

	return 0
}

func runConfigTokenRehash(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("token rehash", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config token rehash <name> [--config PATH] [--config-dir PATH]")
		return 1
	}
	tokenName := positionals[0]

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	tokensPath := filepath.Join(resolvedDir, "tokens.yaml")
	tokensCfg, err := loadTokensFile(tokensPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}
	entry, idx := findTokenByName(tokensCfg.Tokens, tokenName)
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "Token %q not found\n", tokenName)
		return 1
	}
	if entry.ScopesFile == "" {
		fmt.Fprintf(os.Stderr, "Token %q has no scopes_file\n", tokenName)
		return 1
	}

	scopePath := filepath.Join(resolvedDir, filepath.FromSlash(entry.ScopesFile))
	h, err := config.ComputeBlake3Hash(scopePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash scope file: %v\n", err)
		return 1
	}

	oldHash := entry.ScopesHash
	entry.ScopesHash = "blake3:" + h
	tokensCfg.Tokens[idx] = entry
	if err := writeTokensFile(tokensPath, tokensCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write tokens file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}

	if format == "json" {
		out := map[string]any{
			"token":      tokenName,
			"old_hash":   oldHash,
			"new_hash":   entry.ScopesHash,
			"updated":    tokensPath,
			"validation": validation,
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
		return code
	}

	fmt.Printf("Token: %s\n", tokenName)
	fmt.Printf("Scope file: %s\n", scopePath)
	fmt.Printf("Old hash: %s\n", oldHash)
	fmt.Printf("New hash: %s\n", entry.ScopesHash)
	fmt.Printf("Updated: %s\n", tokensPath)
	printValidationSummary(validation)
	return code
}

func runConfigTokenDelete(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("token delete", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config token delete <name> [--config PATH] [--config-dir PATH]")
		return 1
	}
	tokenName := positionals[0]

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	tokensPath := filepath.Join(resolvedDir, "tokens.yaml")
	tokensCfg, err := loadTokensFile(tokensPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}

	_, idx := findTokenByName(tokensCfg.Tokens, tokenName)
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "Token %q not found\n", tokenName)
		return 1
	}
	deleted := tokensCfg.Tokens[idx]
	tokensCfg.Tokens = append(tokensCfg.Tokens[:idx], tokensCfg.Tokens[idx+1:]...)

	if err := writeTokensFile(tokensPath, tokensCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write tokens file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}

	if format == "json" {
		out := map[string]any{
			"deleted":     tokenName,
			"updated":     tokensPath,
			"scopes_file": deleted.ScopesFile,
			"validation":  validation,
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
		return code
	}

	fmt.Printf("Backup: %s.bak\n", tokensPath)
	fmt.Printf("Deleted: %s from %s\n", tokenName, tokensPath)
	if deleted.ScopesFile != "" {
		fmt.Printf("Scope file preserved: %s\n", deleted.ScopesFile)
	}
	printValidationSummary(validation)
	return code
}

func runConfigScopeAdd(args []string) int {
	return runConfigScopeMutation("add", args)
}

func runConfigScopeRemove(args []string) int {
	return runConfigScopeMutation("remove", args)
}

func runConfigScopeSet(args []string) int {
	return runConfigScopeMutation("set", args)
}

func runConfigScopeMutation(mode string, args []string) int {
	var configPath, configDir, format string

	fs := flag.NewFlagSet("scope "+mode, flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if len(positionals) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: ductile config scope %s <token> <scope-or-scope-list> [--config PATH] [--config-dir PATH]\n", mode)
		return 1
	}
	name := positionals[0]
	scopeArg := positionals[1]

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	tokensPath := filepath.Join(resolvedDir, "tokens.yaml")
	tokensCfg, err := loadTokensFile(tokensPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tokens: %v\n", err)
		return 1
	}
	entry, idx := findTokenByName(tokensCfg.Tokens, name)
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "Token %q not found\n", name)
		return 1
	}
	if entry.ScopesFile == "" {
		fmt.Fprintf(os.Stderr, "Token %q has no scopes_file\n", name)
		return 1
	}

	scopeFilePath := filepath.Join(resolvedDir, filepath.FromSlash(entry.ScopesFile))
	doc, err := loadScopeDoc(scopeFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load scope file: %v\n", err)
		return 1
	}

	targetScopes := parseCSVScopes(scopeArg)
	if len(targetScopes) == 0 {
		fmt.Fprintln(os.Stderr, "Error: scope value is empty")
		return 1
	}

	switch mode {
	case "add":
		doc.Scopes = append(doc.Scopes, targetScopes...)
		doc.Scopes = uniqueStrings(doc.Scopes)
	case "remove":
		toRemove := make(map[string]struct{}, len(targetScopes))
		for _, s := range targetScopes {
			toRemove[s] = struct{}{}
		}
		kept := make([]string, 0, len(doc.Scopes))
		for _, s := range doc.Scopes {
			if _, drop := toRemove[s]; !drop {
				kept = append(kept, s)
			}
		}
		doc.Scopes = kept
	case "set":
		doc.Scopes = uniqueStrings(targetScopes)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported scope mutation mode: %s\n", mode)
		return 1
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode scope file: %v\n", err)
		return 1
	}
	raw = append(raw, '\n')
	if err := writeFileAtomicWithBackup(scopeFilePath, raw, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write scope file: %v\n", err)
		return 1
	}

	h, err := config.ComputeBlake3Hash(scopeFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash scope file: %v\n", err)
		return 1
	}
	entry.ScopesHash = "blake3:" + h
	tokensCfg.Tokens[idx] = entry
	if err := writeTokensFile(tokensPath, tokensCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update tokens: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}

	if format == "json" {
		out := map[string]any{
			"token":       name,
			"mode":        mode,
			"scopes":      doc.Scopes,
			"scopes_file": entry.ScopesFile,
			"scopes_hash": entry.ScopesHash,
			"validation":  validation,
		}
		rawOut, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(rawOut))
		return code
	}

	fmt.Printf("Token: %s\n", name)
	fmt.Printf("Updated: %s\n", entry.ScopesFile)
	fmt.Printf("Scopes hash: %s\n", entry.ScopesHash)
	fmt.Printf("Scopes: %s\n", strings.Join(doc.Scopes, ", "))
	printValidationSummary(validation)
	return code
}

func runConfigScopeValidate(args []string) int {
	var configPath, configDir, format string

	fs := flag.NewFlagSet("scope validate", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config scope validate <scope> [--config PATH] [--config-dir PATH]")
		return 1
	}
	scope := strings.TrimSpace(positionals[0])
	if scope == "" {
		fmt.Fprintln(os.Stderr, "Error: scope cannot be empty")
		return 1
	}

	resolvedPath, _, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load error: %v\n", err)
		return 1
	}
	registry, err := discoverRegistry(cfg, resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Plugin discovery error: %v\n", err)
		return 1
	}

	details, err := validateScopeAgainstRegistry(scope, registry)
	if format == "json" {
		out := map[string]any{
			"scope":   scope,
			"valid":   err == nil,
			"details": details,
		}
		if err != nil {
			out["error"] = err.Error()
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
		if err != nil {
			return 1
		}
		return 0
	}

	fmt.Printf("Scope: %s\n", scope)
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		return 1
	}
	for _, line := range details {
		fmt.Printf("%s\n", line)
	}
	fmt.Println("✓ Valid")
	return 0
}

func resolveConfigTarget(configPath, configDir string) (string, string, error) {
	if configPath != "" && configDir != "" {
		return "", "", errors.New("use only one of --config or --config-dir")
	}

	target := configPath
	if configDir != "" {
		target = configDir
	}
	if target == "" {
		discovered, err := config.DiscoverConfigDir()
		if err != nil {
			return "", "", err
		}
		target = discovered
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}

	info, err := os.Stat(absTarget)
	if err != nil {
		return "", "", fmt.Errorf("config target not found: %w", err)
	}

	if info.IsDir() {
		if _, err := os.Stat(filepath.Join(absTarget, "config.yaml")); err != nil {
			return "", "", fmt.Errorf("config.yaml not found in %s", absTarget)
		}
		return absTarget, absTarget, nil
	}

	return absTarget, filepath.Dir(absTarget), nil
}

func validateConfigAtPath(configPath string) (*doctor.Result, int, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, 1, err
	}
	registry, err := discoverRegistry(cfg, configPath)
	if err != nil {
		return nil, 1, err
	}
	result := doctor.New(cfg, registry).Validate()
	if !result.Valid {
		return result, 1, nil
	}
	if len(result.Warnings) > 0 {
		return result, 2, nil
	}
	return result, 0, nil
}

func discoverRegistry(cfg *config.Config, configPath string) (*plugin.Registry, error) {
	pluginsDir := cfg.PluginsDir
	if !filepath.IsAbs(pluginsDir) {
		baseDir := configPath
		if cfg.ConfigDir != "" {
			baseDir = cfg.ConfigDir
		} else {
			if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
				baseDir = filepath.Dir(configPath)
			}
		}
		pluginsDir = filepath.Join(baseDir, pluginsDir)
	}
	return plugin.Discover(pluginsDir, func(level, msg string, args ...any) {})
}

func printValidationSummary(result *doctor.Result) {
	if result == nil {
		return
	}
	if !result.Valid {
		fmt.Printf("Validation: failed (%d error(s), %d warning(s))\n", len(result.Errors), len(result.Warnings))
		for _, issue := range result.Errors {
			if issue.Field != "" {
				fmt.Printf("  ERROR [%s] %s: %s\n", issue.Category, issue.Field, issue.Message)
			} else {
				fmt.Printf("  ERROR [%s] %s\n", issue.Category, issue.Message)
			}
		}
		for _, issue := range result.Warnings {
			if issue.Field != "" {
				fmt.Printf("  WARN  [%s] %s: %s\n", issue.Category, issue.Field, issue.Message)
			} else {
				fmt.Printf("  WARN  [%s] %s\n", issue.Category, issue.Message)
			}
		}
		return
	}

	if len(result.Warnings) == 0 {
		fmt.Println("Validation: ✓ All checks passed")
		return
	}
	fmt.Printf("Validation: ✓ passed with %d warning(s)\n", len(result.Warnings))
	for _, issue := range result.Warnings {
		if issue.Field != "" {
			fmt.Printf("  WARN  [%s] %s: %s\n", issue.Category, issue.Field, issue.Message)
		} else {
			fmt.Printf("  WARN  [%s] %s\n", issue.Category, issue.Message)
		}
	}
}

func refreshConfigIntegrity(configDir string) error {
	files, err := config.DiscoverConfigFiles(configDir)
	if err != nil {
		_, legacyErr := config.GenerateChecksumsWithReport(configDir, []string{"tokens.yaml", "webhooks.yaml"}, false)
		if legacyErr != nil {
			return err
		}
		return nil
	}
	return config.GenerateChecksumsFromDiscovery(files, false)
}

func loadTokensFile(path string) (*config.TokensFileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &config.TokensFileConfig{Tokens: []config.TokenEntry{}}, nil
		}
		return nil, err
	}
	var out config.TokensFileConfig
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Tokens == nil {
		out.Tokens = []config.TokenEntry{}
	}
	return &out, nil
}

func writeTokensFile(path string, cfg *config.TokensFileConfig) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeFileAtomicWithBackup(path, raw, 0o600)
}

func writeFileAtomicWithBackup(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if current, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", current, mode); err != nil {
			return err
		}
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func parseScopesInput(scopesArg, scopesFile string) ([]string, error) {
	if scopesArg != "" {
		return parseCSVScopes(scopesArg), nil
	}

	var raw []byte
	var err error
	if scopesFile == "-" {
		raw, err = ioReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(scopesFile)
	}
	if err != nil {
		return nil, err
	}

	doc := scopeDoc{}
	if err := json.Unmarshal(raw, &doc); err == nil && len(doc.Scopes) > 0 {
		return uniqueStrings(doc.Scopes), nil
	}

	var direct []string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return uniqueStrings(direct), nil
	}

	return nil, errors.New("scopes file must be JSON object with 'scopes' array or a JSON array")
}

func loadScopeDoc(path string) (*scopeDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &scopeDoc{Scopes: []string{}}, nil
		}
		return nil, err
	}

	var doc scopeDoc
	if err := json.Unmarshal(raw, &doc); err == nil {
		doc.Scopes = uniqueStrings(doc.Scopes)
		return &doc, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return &scopeDoc{Scopes: uniqueStrings(arr)}, nil
	}
	return nil, errors.New("invalid scope file JSON")
}

func loadTokenScopes(path string) ([]string, error) {
	doc, err := loadScopeDoc(path)
	if err != nil {
		return nil, err
	}
	return doc.Scopes, nil
}

func findTokenByName(tokens []config.TokenEntry, name string) (config.TokenEntry, int) {
	for i, t := range tokens {
		if t.Name == name {
			return t, i
		}
	}
	return config.TokenEntry{}, -1
}

func parseCSVScopes(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func tokenEnvVarName(name string) string {
	var b strings.Builder
	for _, ch := range strings.ToUpper(name) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" {
		result = "DUCTILE_TOKEN"
	}
	if !strings.HasSuffix(result, "_TOKEN") {
		result += "_TOKEN"
	}
	return result
}

func generateSecureToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func validateScopeAgainstRegistry(scope string, registry *plugin.Registry) ([]string, error) {
	if scope == "*" {
		return []string{"Type: wildcard admin scope"}, nil
	}

	parts := strings.SplitN(scope, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid scope format %q (expected resource:access)", scope)
	}

	resource := parts[0]
	access := parts[1]

	if resource == "read" || resource == "trigger" || resource == "admin" {
		return []string{
			fmt.Sprintf("Type: low-level scope"),
			fmt.Sprintf("Resource: %s", resource),
			fmt.Sprintf("Selector: %s", access),
		}, nil
	}

	p, ok := registry.Get(resource)
	if !ok {
		if resource == "jobs" || resource == "events" || resource == "healthz" || resource == "queue" {
			return []string{
				fmt.Sprintf("Resource: %s", resource),
				fmt.Sprintf("Access: %s", access),
			}, nil
		}
		return nil, fmt.Errorf("plugin %q not found in plugins directory", resource)
	}

	lines := []string{
		fmt.Sprintf("Plugin: %s (found)", resource),
	}

	switch {
	case access == "ro":
		lines = append(lines, "Access: ro (read-only)")
		readCommands := p.GetReadCommands()
		if len(readCommands) == 0 {
			lines = append(lines, "Expands to: no explicit read commands in manifest")
		} else {
			sort.Strings(readCommands)
			for _, cmd := range readCommands {
				lines = append(lines, fmt.Sprintf("  - trigger:%s:%s", resource, cmd))
			}
		}
	case access == "rw":
		lines = append(lines, "Access: rw (read-write)")
	case strings.HasPrefix(access, "allow:"):
		cmd := strings.TrimPrefix(access, "allow:")
		if cmd == "" {
			return nil, fmt.Errorf("allow scope missing command")
		}
		if cmd != "*" && !p.SupportsCommand(cmd) {
			return nil, fmt.Errorf("plugin %q has no command %q", resource, cmd)
		}
		lines = append(lines, fmt.Sprintf("Access: allow:%s", cmd))
	case strings.HasPrefix(access, "deny:"):
		cmd := strings.TrimPrefix(access, "deny:")
		if cmd == "" {
			return nil, fmt.Errorf("deny scope missing command")
		}
		if cmd != "*" && !p.SupportsCommand(cmd) {
			return nil, fmt.Errorf("plugin %q has no command %q", resource, cmd)
		}
		lines = append(lines, fmt.Sprintf("Access: deny:%s", cmd))
	default:
		return nil, fmt.Errorf("invalid scope access type %q (expected ro, rw, allow:cmd, deny:cmd)", access)
	}

	return lines, nil
}

func splitFlagsAndPositionals(args []string, takesValue map[string]bool) ([]string, []string) {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		if takesValue[arg] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return flags, positionals
}

func ioReadAll(f *os.File) ([]byte, error) {
	return io.ReadAll(f)
}

func runConfigPlugin(args []string) int {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigPluginHelp()
		return 0
	}

	action := args[0]
	actionArgs := args[1:]
	switch action {
	case "list":
		return runConfigPluginList(actionArgs)
	case "show":
		return runConfigPluginShow(actionArgs)
	case "set":
		return runConfigPluginSet(actionArgs)
	case "help":
		printConfigPluginHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config plugin action: %s\n", action)
		return 1
	}
}

func runConfigPluginList(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("plugin list", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	resolvedPath, _, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load error: %v\n", err)
		return 1
	}
	registry, err := discoverRegistry(cfg, resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Plugin discovery error: %v\n", err)
		return 1
	}

	type row struct {
		Name       string   `json:"name"`
		Configured bool     `json:"configured"`
		Commands   []string `json:"commands,omitempty"`
	}
	rows := make([]row, 0, len(registry.All()))
	for name, discovered := range registry.All() {
		cmds := make([]string, 0, len(discovered.Commands))
		for _, c := range discovered.Commands {
			cmds = append(cmds, c.Name+":"+string(c.Type))
		}
		sort.Strings(cmds)
		_, configured := cfg.Plugins[name]
		rows = append(rows, row{Name: name, Configured: configured, Commands: cmds})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	if format == "json" {
		raw, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(raw))
		return 0
	}

	fmt.Printf("Discovered plugins in %s:\n", cfg.PluginsDir)
	if len(rows) == 0 {
		fmt.Println("  (none)")
		return 0
	}
	for _, r := range rows {
		state := "not configured"
		if r.Configured {
			state = "configured"
		}
		fmt.Printf("\n%s (%s)\n", r.Name, state)
		if len(r.Commands) > 0 {
			fmt.Printf("  Commands: %s\n", strings.Join(r.Commands, ", "))
		}
	}
	return 0
}

func runConfigPluginShow(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("plugin show", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config plugin show <name> [--config PATH] [--config-dir PATH]")
		return 1
	}
	name := positionals[0]

	resolvedPath, _, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load error: %v\n", err)
		return 1
	}
	pluginCfg, ok := cfg.Plugins[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "Plugin %q not found in config\n", name)
		return 1
	}

	if format == "json" {
		raw, _ := json.MarshalIndent(pluginCfg, "", "  ")
		fmt.Println(string(raw))
		return 0
	}
	raw, _ := yaml.Marshal(pluginCfg)
	fmt.Print(string(raw))
	return 0
}

func runConfigPluginSet(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("plugin set", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
		"--format":     true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config plugin set <name> <path> <value> [--config PATH] [--config-dir PATH]")
		return 1
	}
	name := positionals[0]
	fieldPath := positionals[1]
	value := positionals[2]

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load error: %v\n", err)
		return 1
	}

	targetFile, pluginCfg, err := loadOrCreatePluginConfig(cfg, resolvedDir, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve plugin config file: %v\n", err)
		return 1
	}

	pluginMap := map[string]any{}
	encoded, _ := yaml.Marshal(pluginCfg)
	_ = yaml.Unmarshal(encoded, &pluginMap)
	setNestedMapValue(pluginMap, strings.Split(fieldPath, "."), parseScalarValue(value))

	updatedRaw, _ := yaml.Marshal(pluginMap)
	var updated config.PluginConf
	if err := yaml.Unmarshal(updatedRaw, &updated); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid plugin update: %v\n", err)
		return 1
	}

	fileCfg := config.PluginsFileConfig{Plugins: map[string]config.PluginConf{name: updated}}
	raw, err := yaml.Marshal(fileCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode plugin file: %v\n", err)
		return 1
	}
	if err := writeFileAtomicWithBackup(targetFile, raw, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write plugin file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	if format == "json" {
		out := map[string]any{
			"plugin":     name,
			"path":       fieldPath,
			"value":      value,
			"updated":    targetFile,
			"validation": validation,
		}
		rawOut, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(rawOut))
		return code
	}

	fmt.Printf("Updated: %s\n", targetFile)
	fmt.Printf("Plugin: %s\n", name)
	fmt.Printf("Changed: %s = %s\n", fieldPath, value)
	printValidationSummary(validation)
	return code
}

func runConfigRoute(args []string) int {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigRouteHelp()
		return 0
	}

	action := args[0]
	actionArgs := args[1:]
	switch action {
	case "list":
		return runConfigRouteList(actionArgs)
	case "add":
		return runConfigRouteAdd(actionArgs)
	case "remove":
		return runConfigRouteRemove(actionArgs)
	case "help":
		printConfigRouteHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config route action: %s\n", action)
		return 1
	}
}

func runConfigRouteList(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("route list", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	_, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	routesFile := filepath.Join(resolvedDir, "routes.yaml")
	routesCfg, err := loadRoutesFile(routesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load routes: %v\n", err)
		return 1
	}

	if format == "json" {
		raw, _ := json.MarshalIndent(routesCfg.Routes, "", "  ")
		fmt.Println(string(raw))
		return 0
	}
	fmt.Printf("Routes in %s:\n", routesFile)
	if len(routesCfg.Routes) == 0 {
		fmt.Println("  (none)")
		return 0
	}
	for _, route := range routesCfg.Routes {
		fmt.Printf("  %s:%s -> %s\n", route.From, route.EventType, route.To)
	}
	return 0
}

func runConfigRouteAdd(args []string) int {
	return runConfigRouteMutation("add", args)
}

func runConfigRouteRemove(args []string) int {
	return runConfigRouteMutation("remove", args)
}

func runConfigRouteMutation(mode string, args []string) int {
	var configPath, configDir, format string
	var from, eventType, to string
	fs := flag.NewFlagSet("route "+mode, flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	fs.StringVar(&from, "from", "", "Source plugin")
	fs.StringVar(&eventType, "event", "", "Source event type")
	fs.StringVar(&to, "to", "", "Target plugin")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if from == "" || eventType == "" || to == "" {
		fmt.Fprintf(os.Stderr, "Usage: ductile config route %s --from <plugin> --event <event_type> --to <plugin>\n", mode)
		return 1
	}

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	routesFile := filepath.Join(resolvedDir, "routes.yaml")
	routesCfg, err := loadRoutesFile(routesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load routes: %v\n", err)
		return 1
	}

	route := config.RouteConfig{From: from, EventType: eventType, To: to}
	switch mode {
	case "add":
		if !containsRoute(routesCfg.Routes, route) {
			routesCfg.Routes = append(routesCfg.Routes, route)
		}
	case "remove":
		filtered := make([]config.RouteConfig, 0, len(routesCfg.Routes))
		removed := false
		for _, existing := range routesCfg.Routes {
			if existing.From == route.From && existing.EventType == route.EventType && existing.To == route.To {
				removed = true
				continue
			}
			filtered = append(filtered, existing)
		}
		if !removed {
			fmt.Fprintf(os.Stderr, "Route not found: %s:%s -> %s\n", from, eventType, to)
			return 1
		}
		routesCfg.Routes = filtered
	}

	if err := writeRoutesFile(routesFile, routesCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write routes file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	if format == "json" {
		out := map[string]any{
			"action":     mode,
			"route":      route,
			"updated":    routesFile,
			"validation": validation,
		}
		rawOut, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(rawOut))
		return code
	}

	actionLabel := "Added"
	if mode == "remove" {
		actionLabel = "Removed"
	}
	fmt.Printf("%s route: %s:%s -> %s\n", actionLabel, from, eventType, to)
	fmt.Printf("Updated: %s\n", routesFile)
	printValidationSummary(validation)
	return code
}

func runConfigWebhook(args []string) int {
	if len(args) == 0 || isHelpToken(args[0]) {
		printConfigWebhookHelp()
		return 0
	}

	action := args[0]
	actionArgs := args[1:]
	switch action {
	case "list":
		return runConfigWebhookList(actionArgs)
	case "add":
		return runConfigWebhookAdd(actionArgs)
	case "help":
		printConfigWebhookHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown config webhook action: %s\n", action)
		return 1
	}
}

func runConfigWebhookList(args []string) int {
	var configPath, configDir, format string
	fs := flag.NewFlagSet("webhook list", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	_, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	webhooksFile := filepath.Join(resolvedDir, "webhooks.yaml")
	webhooksCfg, err := loadWebhooksFile(webhooksFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load webhooks: %v\n", err)
		return 1
	}

	if format == "json" {
		raw, _ := json.MarshalIndent(webhooksCfg.Webhooks, "", "  ")
		fmt.Println(string(raw))
		return 0
	}
	fmt.Printf("Webhooks in %s:\n", webhooksFile)
	if len(webhooksCfg.Webhooks) == 0 {
		fmt.Println("  (none)")
		return 0
	}
	for _, hook := range webhooksCfg.Webhooks {
		name := hook.Name
		if name == "" {
			name = hook.Path
		}
		fmt.Printf("\n%s\n", name)
		fmt.Printf("  Path: %s\n", hook.Path)
		fmt.Printf("  Plugin: %s\n", hook.Plugin)
		if hook.SecretRef != "" {
			fmt.Printf("  SecretRef: %s\n", hook.SecretRef)
		} else if hook.Secret != "" {
			fmt.Printf("  Secret: %s\n", hook.Secret)
		}
	}
	return 0
}

func runConfigWebhookAdd(args []string) int {
	var configPath, configDir, format string
	var name, path, pluginName, secret, secretRef, signatureHeader, maxBodySize string
	fs := flag.NewFlagSet("webhook add", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	fs.StringVar(&name, "name", "", "Webhook name")
	fs.StringVar(&path, "path", "", "Webhook path (e.g. /webhook/github)")
	fs.StringVar(&pluginName, "plugin", "", "Target plugin")
	fs.StringVar(&secret, "secret", "", "Webhook secret value")
	fs.StringVar(&secretRef, "secret-ref", "", "Webhook secret reference")
	fs.StringVar(&signatureHeader, "signature-header", "X-Signature-256", "Signature header name")
	fs.StringVar(&maxBodySize, "max-body-size", "1MB", "Maximum request body size")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if path == "" || pluginName == "" {
		fmt.Fprintln(os.Stderr, "Usage: ductile config webhook add --path <path> --plugin <name> [--name <name>] [--secret <value> | --secret-ref <value>]")
		return 1
	}
	if secret == "" && secretRef == "" {
		fmt.Fprintln(os.Stderr, "Error: one of --secret or --secret-ref is required")
		return 1
	}
	if secret != "" && secretRef != "" {
		fmt.Fprintln(os.Stderr, "Error: use only one of --secret or --secret-ref")
		return 1
	}

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	webhooksFile := filepath.Join(resolvedDir, "webhooks.yaml")
	webhooksCfg, err := loadWebhooksFile(webhooksFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load webhooks: %v\n", err)
		return 1
	}
	for _, existing := range webhooksCfg.Webhooks {
		if existing.Path == path {
			fmt.Fprintf(os.Stderr, "Webhook path %q already exists\n", path)
			return 1
		}
		if name != "" && existing.Name == name {
			fmt.Fprintf(os.Stderr, "Webhook name %q already exists\n", name)
			return 1
		}
	}

	entry := config.WebhookEndpoint{
		Name:            name,
		Path:            path,
		Plugin:          pluginName,
		Secret:          secret,
		SecretRef:       secretRef,
		SignatureHeader: signatureHeader,
		MaxBodySize:     maxBodySize,
	}
	webhooksCfg.Webhooks = append(webhooksCfg.Webhooks, entry)
	if err := writeWebhooksFile(webhooksFile, webhooksCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write webhooks file: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	if format == "json" {
		out := map[string]any{
			"webhook":    entry,
			"updated":    webhooksFile,
			"validation": validation,
		}
		rawOut, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(rawOut))
		return code
	}

	fmt.Printf("Added webhook: %s\n", path)
	fmt.Printf("Path: %s -> %s\n", path, pluginName)
	fmt.Printf("Updated: %s\n", webhooksFile)
	printValidationSummary(validation)
	return code
}

func runConfigInit(args []string) int {
	var configDir string
	var force bool
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.BoolVar(&force, "force", false, "Overwrite existing files")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to resolve home directory: %v\n", err)
			return 1
		}
		configDir = filepath.Join(homeDir, ".config", "ductile")
	}

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create config directory: %v\n", err)
		return 1
	}
	if err := os.Chmod(configDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set permissions on config directory: %v\n", err)
		return 1
	}

	created := []string{}
	writeIfNeeded := func(path, content string, mode os.FileMode) error {
		if _, err := os.Stat(path); err == nil && !force {
			return nil
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			return err
		}
		created = append(created, path)
		return nil
	}

	pluginsDir := filepath.Join(configDir, "plugins")
	pipelinesDir := filepath.Join(configDir, "pipelines")
	scopesDir := filepath.Join(configDir, "scopes")
	for _, dir := range []string{pluginsDir, pipelinesDir, scopesDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create directory %s: %v\n", dir, err)
			return 1
		}
		_ = os.Chmod(dir, 0o700)
	}

	configYAML := `service:
  name: ductile
  tick_interval: 60s
  log_level: info
  log_format: json
state:
  path: ./data/ductile.db
api:
  enabled: false
  listen: "127.0.0.1:8080"
plugins_dir: ` + pluginsDir + `
plugins: {}
`
	if err := writeIfNeeded(filepath.Join(configDir, "config.yaml"), configYAML, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config.yaml: %v\n", err)
		return 1
	}
	if err := writeIfNeeded(filepath.Join(configDir, "routes.yaml"), "routes: []\n", 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write routes.yaml: %v\n", err)
		return 1
	}
	if err := writeIfNeeded(filepath.Join(configDir, "webhooks.yaml"), "webhooks: []\n", 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write webhooks.yaml: %v\n", err)
		return 1
	}
	if err := writeIfNeeded(filepath.Join(configDir, "tokens.yaml"), "tokens: []\n", 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write tokens.yaml: %v\n", err)
		return 1
	}

	if err := refreshConfigIntegrity(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate checksums: %v\n", err)
		return 1
	}

	fmt.Printf("Initialized: %s\n", configDir)
	for _, path := range created {
		fmt.Printf("Created: %s\n", path)
	}
	validation, code, err := validateConfigAtPath(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	printValidationSummary(validation)
	return code
}

func runConfigBackup(args []string) int {
	var configPath, configDir, outputPath string
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&outputPath, "output", "", "Backup archive output path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	_, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	if outputPath == "" {
		stamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
		outputPath = filepath.Join(resolvedDir, "backup-"+stamp+".tar.gz")
	}

	files, err := createConfigBackup(resolvedDir, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
		return 1
	}

	fmt.Printf("Created backup: %s\n", outputPath)
	fmt.Printf("Includes: %s\n", strings.Join(files, ", "))
	return 0
}

func runConfigRestore(args []string) int {
	var configPath, configDir string
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	flagArgs, positionals := splitFlagsAndPositionals(args, map[string]bool{
		"--config":     true,
		"--config-dir": true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ductile config restore <archive.tar.gz> [--config-dir PATH]")
		return 1
	}
	archivePath := positionals[0]

	resolvedPath, resolvedDir, err := resolveRestoreTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}
	if err := restoreConfigBackup(resolvedDir, archivePath); err != nil {
		fmt.Fprintf(os.Stderr, "Restore failed: %v\n", err)
		return 1
	}
	if err := refreshConfigIntegrity(resolvedDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh checksums: %v\n", err)
		return 1
	}

	validation, code, err := validateConfigAtPath(resolvedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed to run: %v\n", err)
		return 1
	}
	fmt.Printf("Restored from: %s\n", archivePath)
	printValidationSummary(validation)
	return code
}

func resolveRestoreTarget(configPath, configDir string) (string, string, error) {
	if configPath != "" || configDir != "" {
		if configPath != "" && configDir != "" {
			return "", "", errors.New("use only one of --config or --config-dir")
		}
		target := configPath
		if configDir != "" {
			target = configDir
		}
		abs, err := filepath.Abs(target)
		if err != nil {
			return "", "", err
		}
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			return abs, filepath.Dir(abs), nil
		}
		return abs, abs, nil
	}
	return resolveConfigTarget(configPath, configDir)
}

func loadOrCreatePluginConfig(cfg *config.Config, configDir, name string) (string, config.PluginConf, error) {
	files, err := config.DiscoverConfigFiles(configDir)
	if err == nil {
		for _, path := range files.Plugins {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return "", config.PluginConf{}, readErr
			}
			fileCfg := config.PluginsFileConfig{}
			if unmarshalErr := yaml.Unmarshal(raw, &fileCfg); unmarshalErr != nil {
				return "", config.PluginConf{}, unmarshalErr
			}
			if pluginCfg, ok := fileCfg.Plugins[name]; ok {
				return path, pluginCfg, nil
			}
		}
	}

	defaultCfg := config.PluginConf{
		Enabled: false,
		Schedule: &config.ScheduleConfig{
			Every: "daily",
		},
	}
	if existing, ok := cfg.Plugins[name]; ok {
		defaultCfg = existing
	}

	path := filepath.Join(configDir, "plugins", name+".yaml")
	return path, defaultCfg, nil
}

func parseScalarValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	if i, err := strconv.Atoi(trimmed); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}
	return raw
}

func setNestedMapValue(root map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := root
	for i := 0; i < len(path)-1; i++ {
		segment := strings.TrimSpace(path[i])
		next, ok := current[segment]
		if !ok {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		asMap, ok := next.(map[string]any)
		if !ok {
			child := map[string]any{}
			current[segment] = child
			current = child
			continue
		}
		current = asMap
	}
	current[strings.TrimSpace(path[len(path)-1])] = value
}

func containsRoute(routes []config.RouteConfig, target config.RouteConfig) bool {
	for _, route := range routes {
		if route.From == target.From && route.EventType == target.EventType && route.To == target.To {
			return true
		}
	}
	return false
}

func loadRoutesFile(path string) (*config.RoutesFileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &config.RoutesFileConfig{Routes: []config.RouteConfig{}}, nil
		}
		return nil, err
	}
	cfg := config.RoutesFileConfig{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Routes == nil {
		cfg.Routes = []config.RouteConfig{}
	}
	return &cfg, nil
}

func writeRoutesFile(path string, cfg *config.RoutesFileConfig) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeFileAtomicWithBackup(path, raw, 0o644)
}

func loadWebhooksFile(path string) (*config.WebhooksFileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &config.WebhooksFileConfig{Webhooks: []config.WebhookEndpoint{}}, nil
		}
		return nil, err
	}
	cfg := config.WebhooksFileConfig{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Webhooks == nil {
		cfg.Webhooks = []config.WebhookEndpoint{}
	}
	return &cfg, nil
}

func writeWebhooksFile(path string, cfg *config.WebhooksFileConfig) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeFileAtomicWithBackup(path, raw, 0o600)
}

func createConfigBackup(configDir, outputPath string) ([]string, error) {
	items := []string{"config.yaml", "routes.yaml", "tokens.yaml", "webhooks.yaml", ".checksums", "plugins", "pipelines", "scopes"}
	archiveFile, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	gz := gzip.NewWriter(archiveFile)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	included := []string{}
	for _, rel := range items {
		abs := filepath.Join(configDir, rel)
		info, err := os.Stat(abs)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			err = filepath.Walk(abs, func(path string, entryInfo os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				relPath, _ := filepath.Rel(configDir, path)
				header, err := tar.FileInfoHeader(entryInfo, "")
				if err != nil {
					return err
				}
				header.Name = filepath.ToSlash(relPath)
				if err := tw.WriteHeader(header); err != nil {
					return err
				}
				if entryInfo.IsDir() {
					return nil
				}
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()
				_, err = io.Copy(tw, file)
				return err
			})
			if err != nil {
				return nil, err
			}
			included = append(included, rel+"/")
			continue
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		file, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
		included = append(included, rel)
	}

	sort.Strings(included)
	return included, nil
}

func restoreConfigBackup(configDir, archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") {
			return fmt.Errorf("invalid archive path: %s", header.Name)
		}
		dest := filepath.Join(configDir, cleanName)
		if !strings.HasPrefix(dest, filepath.Clean(configDir)+string(os.PathSeparator)) && filepath.Clean(dest) != filepath.Clean(configDir) {
			return fmt.Errorf("invalid archive entry destination: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				return err
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func printConfigTokenHelp() {
	fmt.Println("Usage: ductile config token <action> [flags]")
	fmt.Println("Actions: create, list, inspect, rehash, delete")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ductile config token create --name github --scopes \"read:jobs,read:events\"")
	fmt.Println("  ductile config token list --format json")
	fmt.Println("  ductile config token inspect github")
}

func printConfigScopeHelp() {
	fmt.Println("Usage: ductile config scope <action> [flags]")
	fmt.Println("Actions: add, remove, set, validate")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ductile config scope add github \"withings:ro\"")
	fmt.Println("  ductile config scope remove github \"read:events\"")
	fmt.Println("  ductile config scope validate \"github-handler:rw\"")
}

func printConfigPluginHelp() {
	fmt.Println("Usage: ductile config plugin <action> [flags]")
	fmt.Println("Actions: list, show, set")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ductile config plugin list")
	fmt.Println("  ductile config plugin show withings")
	fmt.Println("  ductile config plugin set withings schedule.every 2h")
}

func printConfigRouteHelp() {
	fmt.Println("Usage: ductile config route <action> [flags]")
	fmt.Println("Actions: list, add, remove")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ductile config route list")
	fmt.Println("  ductile config route add --from withings --event weight_updated --to slack")
	fmt.Println("  ductile config route remove --from withings --event weight_updated --to slack")
}

func printConfigWebhookHelp() {
	fmt.Println("Usage: ductile config webhook <action> [flags]")
	fmt.Println("Actions: list, add")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ductile config webhook list")
	fmt.Println("  ductile config webhook add --path /webhook/github --plugin github-handler --secret '${GITHUB_WEBHOOK_SECRET}'")
}
