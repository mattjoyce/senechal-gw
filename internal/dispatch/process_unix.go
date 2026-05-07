//go:build darwin || linux || freebsd || openbsd || netbsd

package dispatch

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configurePluginProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminatePluginProcess(cmd *exec.Cmd) error {
	return signalPluginProcessGroup(cmd, syscall.SIGTERM)
}

func killPluginProcess(cmd *exec.Cmd) error {
	return signalPluginProcessGroup(cmd, syscall.SIGKILL)
}

func signalPluginProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// configurePluginProcess starts the plugin with Setpgid=true, which makes
	// the child's process group ID equal to its PID. Use that stable value
	// directly: after SIGTERM the parent process may already be gone, while
	// children in the group still need SIGKILL during the grace-period fallback.
	err := syscall.Kill(-cmd.Process.Pid, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	err = cmd.Process.Signal(signal)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
