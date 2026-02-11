//go:build !darwin && !linux

package storage

import "fmt"

func detectFilesystemType(path string) (string, error) {
	return "", fmt.Errorf("filesystem detection is unsupported on this platform")
}
