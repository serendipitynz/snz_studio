package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// mntReadOnly is MNT_RDONLY from <sys/mount.h>, which package syscall does not export.
const mntReadOnly = 0x1

// Target is the installed .app bundle the running process was launched from.
type Target struct {
	app     string
	staging string // directory beside app holding the extracted new bundle
	staged  string // the new bundle inside staging
}

// LocateTarget finds the running .app and checks it can be replaced by this account
// without elevation. Run it before downloading, so a blocked install fails before
// the user waits for a download it cannot use.
func LocateTarget() (*Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return locateApp(exe)
}

func locateApp(exe string) (*Target, error) {
	macOSDir := filepath.Dir(exe)
	contents := filepath.Dir(macOSDir)
	app := filepath.Dir(contents)
	if filepath.Base(macOSDir) != "MacOS" || filepath.Base(contents) != "Contents" || filepath.Ext(app) != ".app" {
		return nil, &BlockedError{Reason: ReasonNotInstalled, Path: exe}
	}
	// A translocated app runs from a randomized mount under this directory; there is
	// no public API for the check without cgo, and the path is stable across macOS
	// versions that have translocation.
	if strings.Contains(app, "/AppTranslocation/") {
		return nil, &BlockedError{Reason: ReasonTranslocated, Path: app}
	}
	parent := filepath.Dir(app)
	var fs syscall.Statfs_t
	if err := syscall.Statfs(parent, &fs); err != nil {
		return nil, err
	}
	if fs.Flags&mntReadOnly != 0 {
		return nil, &BlockedError{Reason: ReasonReadOnlyVolume, Path: app}
	}
	// The swap renames the bundle into another directory, which needs write access to
	// the bundle itself (its ".." entry changes) as well as to its parent.
	const wOK = 0x2
	if syscall.Access(parent, wOK) != nil || syscall.Access(app, wOK) != nil {
		return nil, &BlockedError{Reason: ReasonNotWritable, Path: app}
	}
	return &Target{app: app}, nil
}

// Prepare extracts the verified archive next to the installed bundle, so that the
// swap in Replace is two renames on one volume. ditto restores the bundle's
// symlinks and extended attributes, which a plain unzip would drop and break the
// code signature with.
func (t *Target) Prepare(archive string) error {
	staging, err := os.MkdirTemp(filepath.Dir(t.app), ".snz-studio-update-")
	if err != nil {
		return err
	}
	if out, err := exec.Command("ditto", "-x", "-k", archive, staging).CombinedOutput(); err != nil {
		os.RemoveAll(staging)
		return fmt.Errorf("updater: extract %s: %v: %s", archive, err, out)
	}
	staged, err := singleApp(staging)
	if err != nil {
		os.RemoveAll(staging)
		return err
	}
	t.staging, t.staged = staging, staged
	return nil
}

func singleApp(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found string
	for _, e := range entries {
		if e.IsDir() && filepath.Ext(e.Name()) == ".app" {
			if found != "" {
				return "", errors.New("updater: the update archive holds more than one .app")
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	if found == "" {
		return "", errors.New("updater: the update archive holds no .app")
	}
	if _, err := os.Stat(filepath.Join(found, "Contents", "MacOS")); err != nil {
		return "", fmt.Errorf("updater: %s is not an app bundle: %w", found, err)
	}
	return found, nil
}

// Replace swaps the prepared bundle in under the installed bundle's name, which the
// user may have changed, and puts the old bundle back if the second rename fails.
// The running process keeps working after its bundle is moved away: the binary
// stays open by inode, and the UI assets are compiled into it.
func (t *Target) Replace() error {
	if t.staged == "" {
		return errors.New("updater: Replace called before Prepare")
	}
	old := filepath.Join(t.staging, "previous-"+filepath.Base(t.app))
	if err := os.Rename(t.app, old); err != nil {
		t.Discard()
		return err
	}
	if err := os.Rename(t.staged, t.app); err != nil {
		if restoreErr := os.Rename(old, t.app); restoreErr != nil {
			return fmt.Errorf("updater: install failed (%v) and the previous app could not be restored: %w", err, restoreErr)
		}
		t.Discard()
		return err
	}
	t.Discard()
	return nil
}

// Discard removes the staging directory and whatever is left in it.
func (t *Target) Discard() {
	if t.staging != "" {
		os.RemoveAll(t.staging)
	}
}

// Relaunch starts the bundle now installed at the target's path. Call it as the
// last step of shutdown: -n starts a new instance even while this one is still
// exiting, rather than activating the one going away.
func (t *Target) Relaunch() error {
	return exec.Command("open", "-n", t.app).Run()
}
