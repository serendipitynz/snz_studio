//go:build !darwin && !windows

package updater

import "errors"

// Target exists so the app builds where no update files are published.
type Target struct{}

func LocateTarget() (*Target, error) {
	return nil, &BlockedError{Reason: ReasonUnsupportedPlatform}
}

func (t *Target) Prepare(string) error { return errors.New("updater: unsupported platform") }
func (t *Target) Replace() error       { return errors.New("updater: unsupported platform") }
func (t *Target) Discard()             {}
func (t *Target) Relaunch() error      { return errors.New("updater: unsupported platform") }
