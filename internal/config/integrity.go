package config

import (
	"fmt"
	"path/filepath"
)

// VerifyIntegrity checks all discovered files against the .checksums manifest.
// High-security mismatches produce errors (hard fail). Operational mismatches produce warnings.
// Returns an IntegrityResult with collected warnings and errors.
func VerifyIntegrity(configDir string, files *ConfigFiles) (*IntegrityResult, error) {
	result := &IntegrityResult{Passed: true}

	checksumPath := filepath.Join(configDir, ".checksums")
	manifest, err := LoadChecksums(configDir)
	if err != nil {
		// No .checksums file at all
		highSec := files.HighSecurityFiles()
		if len(highSec) > 0 {
			result.Passed = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("no .checksums manifest found at %s but high-security files exist; run 'ductile config lock'", checksumPath))
			return result, nil
		}
		// No high-security files and no manifest — just warn
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("no .checksums manifest found at %s; run 'ductile config lock' to enable integrity verification", checksumPath))
		return result, nil
	}

	// Verify each discovered file
	for _, path := range files.AllFiles() {
		tier := files.FileTier(path)
		expectedHash, inManifest := manifest.Hashes[path]

		if !inManifest {
			msg := fmt.Sprintf("file %s not in .checksums manifest", path)
			if tier == TierHighSecurity {
				result.Passed = false
				result.Errors = append(result.Errors, msg)
			} else {
				result.Warnings = append(result.Warnings, msg)
			}
			continue
		}

		actualHash, err := ComputeBlake3Hash(path)
		if err != nil {
			msg := fmt.Sprintf("failed to hash %s: %v", path, err)
			if tier == TierHighSecurity {
				result.Passed = false
				result.Errors = append(result.Errors, msg)
			} else {
				result.Warnings = append(result.Warnings, msg)
			}
			continue
		}

		if actualHash != expectedHash {
			msg := fmt.Sprintf("hash mismatch for %s (expected %s, got %s)", path, expectedHash, actualHash)
			if tier == TierHighSecurity {
				result.Passed = false
				result.Errors = append(result.Errors, msg)
			} else {
				result.Warnings = append(result.Warnings, msg)
			}
		}
	}

	return result, nil
}
