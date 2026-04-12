//go:build darwin

package storage

import (
	"fmt"
	"syscall"
)

func detectFilesystemType(path string) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", fmt.Errorf("statfs %q: %w", path, err)
	}
	fsType, err := int8ArrayToString(stat.Fstypename[:])
	if err != nil {
		return "", fmt.Errorf("decode filesystem type: %w", err)
	}
	return fsType, nil
}

func int8ArrayToString(buf []int8) (string, error) {
	out := make([]byte, 0, len(buf))
	for _, b := range buf {
		if b == 0 {
			break
		}
		if b < 0 {
			return "", fmt.Errorf("non-ascii filesystem type byte")
		}
		out = append(out, byte(b))
	}
	return string(out), nil
}
