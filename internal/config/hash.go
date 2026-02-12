package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zeebo/blake3"
	"gopkg.in/yaml.v3"
)

// HashUpdateFileResult captures checksum generation outcome for a scope file.
type HashUpdateFileResult struct {
	Filename string
	Path     string
	Exists   bool
	Hash     string
}

// HashUpdateReport captures checksum generation details for a config directory.
type HashUpdateReport struct {
	ConfigDir    string
	ChecksumPath string
	Written      bool
	Files        []HashUpdateFileResult
}

// ComputeBlake3Hash computes the BLAKE3 hash of a file.
func ComputeBlake3Hash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	hash := blake3.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// VerifyFileHash verifies a file against an expected BLAKE3 hash.
func VerifyFileHash(filePath, expectedHash string) error {
	actualHash, err := ComputeBlake3Hash(filePath)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch for %s: expected %s, got %s",
			filepath.Base(filePath), expectedHash, actualHash)
	}

	return nil
}

// GenerateChecksums computes BLAKE3 hashes for scope files and writes .checksums.
func GenerateChecksums(configDir string, scopeFiles []string) error {
	_, err := GenerateChecksumsWithReport(configDir, scopeFiles, false)
	return err
}

// GenerateChecksumsWithReport computes scope file hashes and optionally writes .checksums.
// When dryRun is true, it computes hashes and returns report details without writing files.
func GenerateChecksumsWithReport(configDir string, scopeFiles []string, dryRun bool) (*HashUpdateReport, error) {
	manifest := ChecksumManifest{
		Version:     1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Hashes:      make(map[string]string),
	}

	report := &HashUpdateReport{
		ConfigDir:    configDir,
		ChecksumPath: filepath.Join(configDir, ".checksums"),
		Written:      false,
		Files:        make([]HashUpdateFileResult, 0, len(scopeFiles)),
	}

	// Compute hash for each scope file
	for _, filename := range scopeFiles {
		filePath := filepath.Join(configDir, filename)

		// Skip if file doesn't exist (optional files)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			report.Files = append(report.Files, HashUpdateFileResult{
				Filename: filename,
				Path:     filePath,
				Exists:   false,
				Hash:     "",
			})
			continue
		}

		hash, err := ComputeBlake3Hash(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to hash %s: %w", filename, err)
		}

		manifest.Hashes[filename] = hash
		report.Files = append(report.Files, HashUpdateFileResult{
			Filename: filename,
			Path:     filePath,
			Exists:   true,
			Hash:     hash,
		})
	}

	if dryRun {
		return report, nil
	}

	// Write .checksums file
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal checksums: %w", err)
	}

	// Write with restrictive permissions (contains expected hashes)
	if err := os.WriteFile(report.ChecksumPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write checksums: %w", err)
	}
	report.Written = true

	return report, nil
}

// LoadChecksums reads the .checksums file from a config directory.
// Supports v1 (filename keys) and v2 (absolute path keys) manifests.
// v1 manifests are transparently upgraded: filename keys are resolved to absolute paths.
func LoadChecksums(configDir string) (*ChecksumManifest, error) {
	checksumPath := filepath.Join(configDir, ".checksums")

	data, err := os.ReadFile(checksumPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checksums file not found (run 'senechal-gw config lock')")
		}
		return nil, fmt.Errorf("failed to read checksums: %w", err)
	}

	var manifest ChecksumManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse checksums: %w", err)
	}

	switch manifest.Version {
	case 1:
		// Migrate v1 filename keys to absolute paths for uniform lookup
		absDir, _ := filepath.Abs(configDir)
		migrated := make(map[string]string, len(manifest.Hashes))
		for key, hash := range manifest.Hashes {
			if filepath.IsAbs(key) {
				migrated[key] = hash
			} else {
				migrated[filepath.Join(absDir, key)] = hash
			}
		}
		manifest.Hashes = migrated
	case 2:
		// v2 already uses absolute paths — nothing to do
	default:
		return nil, fmt.Errorf("unsupported checksums version: %d", manifest.Version)
	}

	return &manifest, nil
}

// GenerateChecksumsFromDiscovery computes BLAKE3 hashes for all discovered files
// and writes a v2 manifest with absolute path keys.
func GenerateChecksumsFromDiscovery(files *ConfigFiles, dryRun bool) error {
	manifest := ChecksumManifest{
		Version:     2,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Hashes:      make(map[string]string),
	}

	for _, path := range files.AllFiles() {
		hash, err := ComputeBlake3Hash(path)
		if err != nil {
			return fmt.Errorf("failed to hash %s: %w", path, err)
		}
		manifest.Hashes[path] = hash
	}

	if dryRun {
		return nil
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal checksums: %w", err)
	}

	checksumPath := filepath.Join(files.Root, ".checksums")
	if err := os.WriteFile(checksumPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write checksums: %w", err)
	}

	return nil
}

// VerifyScopeFiles verifies all scope files against their checksums.
// Returns error if any file hash doesn't match.
func VerifyScopeFiles(configDir string, manifest *ChecksumManifest, scopeFiles []string) error {
	for _, filename := range scopeFiles {
		filePath := filepath.Join(configDir, filename)

		// Skip if file doesn't exist (optional files like webhooks.yaml)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// But verify it's also not in the manifest
			if _, hasHash := manifest.Hashes[filename]; hasHash {
				return fmt.Errorf("scope file %s is in checksums but missing from disk", filename)
			}
			continue
		}

		// File exists - must have hash in manifest
		expectedHash, ok := manifest.Hashes[filename]
		if !ok {
			return fmt.Errorf("scope file %s has no hash in checksums (run 'senechal-gw config hash-update')", filename)
		}

		// Verify hash
		if err := VerifyFileHash(filePath, expectedHash); err != nil {
			return fmt.Errorf("scope file verification failed: %w\n"+
				"This indicates tampering or unauthorized modification.\n"+
				"If you edited this file intentionally, run: senechal-gw config hash-update", err)
		}
	}

	return nil
}
