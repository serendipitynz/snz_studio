package updater

import "fmt"

// Reasons an update cannot be installed in place. They are part of the binding's
// contract: the UI branches on them to send the user to the releases page instead,
// never on an error's text.
const (
	ReasonUnsupportedPlatform = "unsupportedPlatform"
	// ReasonNotInstalled: the app is not running from an installed copy (a dev
	// build, or the portable files from a CI artifact).
	ReasonNotInstalled = "notInstalled"
	// ReasonTranslocated: macOS runs the app from a randomized read-only path
	// because it was opened where it was downloaded instead of being moved.
	ReasonTranslocated = "translocated"
	// ReasonReadOnlyVolume: the app runs from a read-only volume, such as the
	// mounted .dmg.
	ReasonReadOnlyVolume = "readOnlyVolume"
	// ReasonNotWritable: this account cannot replace the installed files. Asking
	// for an administrator's password to do it anyway is deliberately not offered.
	ReasonNotWritable = "notWritable"
)

// BlockedError reports that the installed copy cannot be updated in place.
type BlockedError struct {
	Reason string
	Path   string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("updater: cannot update %s in place (%s)", e.Path, e.Reason)
}
