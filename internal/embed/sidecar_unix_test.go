//go:build !windows

package embed

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestSidecarDiesWithItsParent SIGKILLs a stand-in for the app — this test binary
// re-run as a helper that launches a sidecar command — and asserts that the
// sidecar's process group empties, as `wails dev` does to the app on each rebuild.
func TestSidecarDiesWithItsParent(t *testing.T) {
	if os.Getenv("SNZ_SIDECAR_PARENT_HELPER") == "1" {
		runSidecarParentHelper()
		return
	}

	helper := exec.Command(os.Args[0], "-test.run=^TestSidecarDiesWithItsParent$")
	helper.Env = append(os.Environ(), "SNZ_SIDECAR_PARENT_HELPER=1")
	stdout, err := helper.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = helper.Process.Kill()
		t.Fatalf("read sidecar pgid: %v", err)
	}
	pgid, err := strconv.Atoi(line[:len(line)-1])
	if err != nil {
		_ = helper.Process.Kill()
		t.Fatalf("parse sidecar pgid %q: %v", line, err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Fatalf("sidecar group not running before the parent is killed: %v", err)
	}
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_ = helper.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("sidecar process group still alive 5s after its parent was SIGKILLed")
}

// runSidecarParentHelper starts a long-running stand-in sidecar, reports its
// process group on stdout, and blocks until it is killed.
func runSidecarParentHelper() {
	cmd := sidecarCommand("/bin/sleep", []string{"60"})
	configureSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(cmd.Process.Pid)
	select {}
}
