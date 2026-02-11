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
	return int8ArrayToString(stat.Fstypename[:]), nil
}

func int8ArrayToString(buf []int8) string {
	out := make([]byte, 0, len(buf))
	for _, b := range buf {
		if b == 0 {
			break
		}
		out = append(out, byte(b))
	}
	return string(out)
}
