package updater

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func makeBundle(t *testing.T, app, marker string) string {
	t.Helper()
	macOS := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(macOS, "SNZ Studio")
	if err := os.WriteFile(exe, []byte(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func blockedReason(err error) string {
	var b *BlockedError
	if errors.As(err, &b) {
		return b.Reason
	}
	return ""
}

func TestLocateAppReasons(t *testing.T) {
	dir := t.TempDir()
	exe := makeBundle(t, filepath.Join(dir, "SNZ Studio.app"), "old")
	if _, err := locateApp(exe); err != nil {
		t.Fatalf("installed bundle rejected: %v", err)
	}
	if got := blockedReason(func() error { _, err := locateApp(filepath.Join(dir, "snz-studio")); return err }()); got != ReasonNotInstalled {
		t.Errorf("bare binary: reason %q", got)
	}
	translocated := makeBundle(t, filepath.Join(dir, "AppTranslocation", "X", "d", "SNZ Studio.app"), "old")
	if got := blockedReason(func() error { _, err := locateApp(translocated); return err }()); got != ReasonTranslocated {
		t.Errorf("translocated: reason %q", got)
	}

	locked := filepath.Join(dir, "locked")
	lockedExe := makeBundle(t, filepath.Join(locked, "SNZ Studio.app"), "old")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if os.Geteuid() != 0 {
		if got := blockedReason(func() error { _, err := locateApp(lockedExe); return err }()); got != ReasonNotWritable {
			t.Errorf("unwritable parent: reason %q", got)
		}
	}
}

func TestLocateAppReadOnlyVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("creates and mounts a disk image")
	}
	src := t.TempDir()
	makeBundle(t, filepath.Join(src, "SNZ Studio.app"), "old")
	dmg := filepath.Join(t.TempDir(), "test.dmg")
	if out, err := exec.Command("hdiutil", "create", "-quiet", "-fs", "HFS+", "-srcfolder", src, "-format", "UDZO", dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil create: %v: %s", err, out)
	}
	mnt := t.TempDir()
	if out, err := exec.Command("hdiutil", "attach", "-quiet", "-readonly", "-nobrowse", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		t.Skipf("hdiutil attach: %v: %s", err, out)
	}
	t.Cleanup(func() { exec.Command("hdiutil", "detach", "-quiet", "-force", mnt).Run() })

	_, err := locateApp(filepath.Join(mnt, "SNZ Studio.app", "Contents", "MacOS", "SNZ Studio"))
	if got := blockedReason(err); got != ReasonReadOnlyVolume {
		t.Fatalf("reason %q (%v)", got, err)
	}
}

func TestPrepareAndReplace(t *testing.T) {
	dir := t.TempDir()
	exe := makeBundle(t, filepath.Join(dir, "Renamed By User.app"), "old")

	// The release archive names its bundle snz-studio.app (build.yml) and is made
	// with ditto, as here.
	src := t.TempDir()
	makeBundle(t, filepath.Join(src, "snz-studio.app"), "new")
	archive := filepath.Join(t.TempDir(), "update.app.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", filepath.Join(src, "snz-studio.app"), archive).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}

	target, err := locateApp(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Prepare(archive); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Fatal("Prepare touched the installed bundle")
	}
	if err := target.Replace(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("installed bundle holds %q after Replace", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("left %d entries beside the app, want only the app", len(entries))
	}
}

func TestPrepareRejectsArchiveWithoutApp(t *testing.T) {
	dir := t.TempDir()
	exe := makeBundle(t, filepath.Join(dir, "SNZ Studio.app"), "old")
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "readme.txt"), []byte("x"), 0o644)
	archive := filepath.Join(t.TempDir(), "update.zip")
	if out, err := exec.Command("ditto", "-c", "-k", src, archive).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}
	target, _ := locateApp(exe)
	if err := target.Prepare(archive); err == nil {
		t.Fatal("accepted an archive without a bundle")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("left the staging directory behind (%d entries)", len(entries))
	}
}
