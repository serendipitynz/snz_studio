//go:build !windows

package embed

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// configureSysProcAttr puts the sidecar in its own process group so the whole group
// can be killed together (llama-server may spawn helpers).
func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the sidecar's process group (negative pid).
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// defaultServerBinaryPath returns the production location of the bundled
// llama-server on macOS: inside the .app bundle's Resources directory, a sibling of
// the Contents/MacOS executable.
func defaultServerBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// <app>.app/Contents/MacOS/<exe> -> <app>.app/Contents/Resources/llama-server
	return filepath.Join(filepath.Dir(exe), "..", "Resources", "llama-server")
}
