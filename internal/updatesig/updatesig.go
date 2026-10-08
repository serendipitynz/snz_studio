// Package updatesig is the contract between the release workflow and the app's
// updater: the shape of latest.json, the bytes an update signature covers, and
// the public key the app trusts. tools/updatesig signs with this package and the
// app verifies with it, so the two cannot drift into different formats.
package updatesig

import (
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// The key is committed and compiled in. Rotating it means only builds made after
// the rotation accept updates signed with the new key (see tools/updatesig).
//
//go:embed update-signing-key.pub
var publicKeyText string

// Platform keys of latest.json, one per update file a release carries.
const (
	PlatformDarwinUniversal = "darwin-universal"
	PlatformWindowsAMD64    = "windows-amd64"
)

// Platforms lists every key a complete latest.json carries.
var Platforms = []string{PlatformDarwinUniversal, PlatformWindowsAMD64}

// Manifest is latest.json. It carries no release notes: it is written while the
// release is still a draft, before its notes are edited by hand, so notes here
// would ship the unedited text. The release page at the version's tag has them.
type Manifest struct {
	Version   string           `json:"version"`
	Platforms map[string]Asset `json:"platforms"`
}

// Asset is one platform's update file. URL is pinned to the release's tag, never
// to releases/latest, which can move to a model release or a later version.
type Asset struct {
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

// PublicKey returns the key compiled into this build.
func PublicKey() (ed25519.PublicKey, error) {
	return ParsePublicKey(publicKeyText)
}

func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("updatesig: public key is not a base64 Ed25519 public key")
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey reads the form UPDATE_SIGNING_KEY holds: the base64 32-byte seed.
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("updatesig: private key is not a base64 Ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(raw), nil
}

func EncodePublicKey(key ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(key)
}

func EncodePrivateKey(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Seed())
}

func Digest(r io.Reader) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// message is what a signature covers. The file is bound by its digest rather than
// signed whole, so neither side has to hold an update file in memory. Binding the
// version and the platform as well means an older signed file cannot be served
// under a newer version number to downgrade an install, nor one platform's file
// under another's key.
func message(version, platform string, digest [sha256.Size]byte) ([]byte, error) {
	if version == "" || platform == "" || strings.ContainsAny(version+platform, "\n\r") {
		return nil, fmt.Errorf("updatesig: invalid version %q or platform %q", version, platform)
	}
	return fmt.Appendf(nil, "snz-studio-update-v1\nversion=%s\nplatform=%s\nsha256=%x\n", version, platform, digest), nil
}

// Sign returns the base64 signature latest.json carries for one update file.
func Sign(key ed25519.PrivateKey, version, platform string, digest [sha256.Size]byte) (string, error) {
	msg, err := message(version, platform, digest)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, msg)), nil
}

func Verify(key ed25519.PublicKey, version, platform string, digest [sha256.Size]byte, signature string) error {
	msg, err := message(version, platform, digest)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || !ed25519.Verify(key, msg, sig) {
		return fmt.Errorf("updatesig: signature does not match the %s file of version %s", platform, version)
	}
	return nil
}
