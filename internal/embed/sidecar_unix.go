//go:build !windows

package embed

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

// parentWatchScript runs llama-server as a job of /bin/sh and SIGKILLs it once the
// shell's parent — this app — is gone. Shutdown's process-group kill cannot run
// when the app itself is SIGKILLed, which is how `wails dev` stops it on every
// rebuild and on exit (v2.16 internal/process.Kill); without this the sidecar was
// left orphaned under launchd. DYLD_LIBRARY_PATH travels under another name
// because SIP strips DYLD_* from the environment of the protected /bin/sh.
const parentWatchScript = `p=$PPID
DYLD_LIBRARY_PATH="$SNZ_SIDECAR_LIB_DIR" "$@" &
c=$!
(while kill -0 "$p" 2>/dev/null; do sleep 1; done; kill -KILL "$c" 2>/dev/null) &
w=$!
wait "$c"
s=$?
kill "$w" 2>/dev/null
exit "$s"`

// sidecarCommand builds the llama-server command wrapped in parentWatchScript.
// The official macOS build loads sibling dylibs via @rpath; the library path keeps
// the dynamic loader finding them regardless of the launch cwd.
func sidecarCommand(binPath string, args []string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", append([]string{"-c", parentWatchScript, "snz-sidecar", binPath}, args...)...)
	cmd.Env = append(os.Environ(), "SNZ_SIDECAR_LIB_DIR="+filepath.Dir(binPath))
	return cmd
}

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

// devServerBinaryPath is the cwd-relative dev fallback (matching the dev data dir
// convention): build/sidecar/<os>-<arch>/llama-server. `wails dev` runs from the
// repo root, so a developer can drop the per-OS sidecar there instead of setting
// SNZ_LLAMA_SERVER_BIN.
func devServerBinaryPath() string {
	return filepath.Join("build", "sidecar", runtime.GOOS+"-"+runtime.GOARCH, "llama-server")
}

// bundledModelPath returns the production location of a model GGUF shipped inside
// the .app bundle's Resources directory (a sibling of the Contents/MacOS
// executable, alongside the bundled llama-server), or "" if it cannot be resolved.
// seedBundledModel copies from here into the per-user models dir on first launch.
func bundledModelPath(fileName string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// <app>.app/Contents/MacOS/<exe> -> <app>.app/Contents/Resources/<fileName>
	return filepath.Join(filepath.Dir(exe), "..", "Resources", fileName)
}
