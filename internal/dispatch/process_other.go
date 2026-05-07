//go:build !(darwin || linux || freebsd || openbsd || netbsd)

package dispatch

import (
	"errors"
	"os"
	"os/exec"
)

func configurePluginProcess(cmd *exec.Cmd) {}

func terminatePluginProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Kill()
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func killPluginProcess(cmd *exec.Cmd) error {
	return terminatePluginProcess(cmd)
}
