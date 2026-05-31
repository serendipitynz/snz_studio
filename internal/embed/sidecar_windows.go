//go:build windows

package embed

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

// createNewProcessGroup is CREATE_NEW_PROCESS_GROUP; it lets the child be killed as
// a group via taskkill /T.
const createNewProcessGroup = 0x00000200

func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

// killProcessGroup terminates the sidecar and any children with taskkill /T /F.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

// defaultServerBinaryPath returns the production location of the bundled
// llama-server.exe on Windows: alongside the app executable.
func defaultServerBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "llama-server.exe")
}

// devServerBinaryPath is the cwd-relative dev fallback: build/sidecar/<os>-<arch>/
// llama-server.exe. `wails dev` runs from the repo root.
func devServerBinaryPath() string {
	return filepath.Join("build", "sidecar", runtime.GOOS+"-"+runtime.GOARCH, "llama-server.exe")
}
