package main

import (
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
	"strings"
	"time"

	"github.com/mattjoyce/ductile/internal/config"
	"github.com/mattjoyce/ductile/internal/doctor"
	"github.com/mattjoyce/ductile/internal/plugin"
	"gopkg.in/yaml.v3"
)

type scopeDoc struct {
	Scopes   []string       `json:"scopes"`
	Metadata scopeDocFields `json:"metadata,omitempty"`
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

	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "Path to config file or directory")
	fs.StringVar(&configDir, "config-dir", "", "Path to config directory")
	fs.StringVar(&name, "name", "", "Token name")
	fs.StringVar(&scopesArg, "scopes", "", "Comma-separated scopes")
	fs.StringVar(&scopesFile, "scopes-file", "", "Path to scopes JSON file (or - for stdin)")
	fs.StringVar(&description, "description", "", "Token description")
	fs.StringVar(&format, "format", "human", "Output format (human, json)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		return 1
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		return 1
	}
	if scopesArg == "" && scopesFile == "" {
		fmt.Fprintln(os.Stderr, "Error: one of --scopes or --scopes-file is required")
		return 1
	}
	if scopesArg != "" && scopesFile != "" {
		fmt.Fprintln(os.Stderr, "Error: use only one of --scopes or --scopes-file")
		return 1
	}

	resolvedPath, resolvedDir, err := resolveConfigTarget(configPath, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve config failed: %v\n", err)
		return 1
	}

	scopes, err := parseScopesInput(scopesArg, scopesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid scopes: %v\n", err)
		return 1
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
	registry, err := plugin.Discover(cfg.PluginsDir, func(level, msg string, args ...interface{}) {})
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
	registry, err := plugin.Discover(cfg.PluginsDir, func(level, msg string, args ...interface{}) {})
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
