//go:build linux

package storage

import (
	"fmt"
	"syscall"
)

const (
	linuxNFSMagic  = 0x6969
	linuxCIFSMagic = 0xFF534D42
	linuxSMBMagic  = 0x517B
	linuxSMB2Magic = 0xFE534D42
)

func detectFilesystemType(path string) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", fmt.Errorf("statfs %q: %w", path, err)
	}

	switch uint64(stat.Type) {
	case linuxNFSMagic:
		return "nfs", nil
	case linuxCIFSMagic:
		return "cifs", nil
	case linuxSMBMagic:
		return "smbfs", nil
	case linuxSMB2Magic:
		return "smb2", nil
	default:
		return fmt.Sprintf("0x%x", uint64(stat.Type)), nil
	}
}
