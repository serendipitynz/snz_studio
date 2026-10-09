package updater

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Target is the directory the installer put the running exe in.
type Target struct {
	dir       string
	installer string
}

// LocateTarget checks the running exe was installed by the per-user installer and
// that this account can write where it lives. Run it before downloading.
func LocateTarget() (*Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	// The portable files from a CI artifact have no uninstaller beside them; running
	// the installer over them would turn a portable copy into an installed one.
	if _, err := os.Stat(filepath.Join(dir, "uninstall.exe")); err != nil {
		return nil, &BlockedError{Reason: ReasonNotInstalled, Path: dir}
	}
	probe, err := os.CreateTemp(dir, ".snz-studio-write-check-")
	if err != nil {
		return nil, &BlockedError{Reason: ReasonNotWritable, Path: dir}
	}
	probe.Close()
	os.Remove(probe.Name())
	return &Target{dir: dir}, nil
}

// Prepare records the verified installer. Nothing is written into the install
// directory here: the installer does that after this process has exited.
func (t *Target) Prepare(installer string) error {
	t.installer = installer
	return nil
}

// Replace is a no-op on Windows; the installer replaces the files once the app
// has exited.
func (t *Target) Replace() error {
	if t.installer == "" {
		return errors.New("updater: Replace called before Prepare")
	}
	return nil
}

func (t *Target) Discard() {}

// Relaunch hands the installed directory to the installer, run silently. /UPDATE
// makes the installer wait for this exe to exit before writing and start the new
// version afterwards (build/windows/installer/project.nsi). Call it as the last
// step of shutdown.
//
// The command line is written out by hand because NSIS reads /D= as the rest of
// the line, unquoted, and must have it last; exec's own quoting would wrap a path
// containing a space in quotes that NSIS then takes as part of the path.
func (t *Target) Relaunch() error {
	cmd := exec.Command(t.installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `"` + t.installer + `" /S /UPDATE /D=` + t.dir,
	}
	return cmd.Start()
}
