// Creates the update signing key and signs a release's update files. Run from the
// repo root:
//
//	go run ./tools/updatesig keygen -out ~/.config/snz-studio/update-signing.key
//	go run ./tools/updatesig check-key [-key-file PATH]
//	go run ./tools/updatesig sign -version 0.2.0 -base-url URL -out latest.json darwin-universal=FILE windows-amd64=FILE
//	go run ./tools/updatesig verify -version 0.2.0 -base-url URL -dir DIR latest.json
//
// The private key is read from UPDATE_SIGNING_KEY (the repository secret) unless
// -key-file names a file, and is never printed. Every check is made against the
// public key internal/updatesig compiles in — the one the shipped app trusts —
// because a private key that does not match it signs files no client accepts,
// and nothing else in the release would notice.
//
// Rotating the key (keygen again, register the new secret, commit the new public
// key) cuts off every build released before the rotation: they only trust the old
// key, so their users have to install one version by hand.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"snzstudio/internal/updatesig"
)

const publicKeyPath = "internal/updatesig/update-signing-key.pub"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "check-key":
		err = checkKey(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: updatesig keygen|check-key|sign|verify [flags] (see tools/updatesig/main.go)")
	os.Exit(2)
}

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "file to write the private key to (must not exist)")
	fs.Parse(args)
	if *out == "" {
		return errors.New("keygen: -out is required")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o700); err != nil {
		return err
	}
	// O_EXCL: overwriting an existing private key would lose the only copy of the
	// key that released builds trust.
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, updatesig.EncodePrivateKey(priv)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(publicKeyPath, []byte(updatesig.EncodePublicKey(pub)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("private key: %s\npublic key:  %s\n", *out, publicKeyPath)
	return nil
}

func loadPrivateKey(keyFile string) (ed25519.PrivateKey, error) {
	text := os.Getenv("UPDATE_SIGNING_KEY")
	source := "UPDATE_SIGNING_KEY"
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		text, source = string(b), keyFile
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("%s is empty", source)
	}
	key, err := updatesig.ParsePrivateKey(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	pub, err := updatesig.PublicKey()
	if err != nil {
		return nil, err
	}
	if !pub.Equal(key.Public()) {
		return nil, fmt.Errorf("%s does not match the public key in %s; clients would reject every signature it makes", source, publicKeyPath)
	}
	return key, nil
}

func checkKey(args []string) error {
	fs := flag.NewFlagSet("check-key", flag.ExitOnError)
	keyFile := fs.String("key-file", "", "read the private key from this file instead of UPDATE_SIGNING_KEY")
	fs.Parse(args)
	if _, err := loadPrivateKey(*keyFile); err != nil {
		return err
	}
	fmt.Println("the private key matches", publicKeyPath)
	return nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	version := fs.String("version", "", "release version without the leading v (0.2.0)")
	baseURL := fs.String("base-url", "", "download URL prefix pinned to the release tag")
	out := fs.String("out", "", "latest.json to write")
	fs.Parse(args)
	if *version == "" || *baseURL == "" || *out == "" {
		return errors.New("sign: -version, -base-url and -out are required")
	}
	key, err := loadPrivateKey("")
	if err != nil {
		return err
	}
	m := updatesig.Manifest{Version: *version, Platforms: map[string]updatesig.Asset{}}
	for _, arg := range fs.Args() {
		platform, file, ok := strings.Cut(arg, "=")
		if !ok {
			return fmt.Errorf("sign: %q is not PLATFORM=FILE", arg)
		}
		digest, err := fileDigest(file)
		if err != nil {
			return err
		}
		sig, err := updatesig.Sign(key, *version, platform, digest)
		if err != nil {
			return err
		}
		m.Platforms[platform] = updatesig.Asset{
			URL:       strings.TrimSuffix(*baseURL, "/") + "/" + filepath.Base(file),
			Signature: sig,
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
}

// verify re-reads latest.json and the files from disk, so it checks what is about
// to be uploaded rather than what sign meant to write.
func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	version := fs.String("version", "", "version latest.json must declare")
	baseURL := fs.String("base-url", "", "URL prefix every entry must start with")
	dir := fs.String("dir", ".", "directory holding the files the URLs name")
	fs.Parse(args)
	if *version == "" || *baseURL == "" || fs.NArg() != 1 {
		return errors.New("verify: -version, -base-url and one latest.json are required")
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	var m updatesig.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("%s: %w", fs.Arg(0), err)
	}
	if m.Version != *version {
		return fmt.Errorf("latest.json declares version %q, expected %q", m.Version, *version)
	}
	pub, err := updatesig.PublicKey()
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(*baseURL, "/") + "/"
	for _, platform := range updatesig.Platforms {
		a, ok := m.Platforms[platform]
		if !ok {
			return fmt.Errorf("latest.json has no %s entry", platform)
		}
		if !strings.HasPrefix(a.URL, prefix) || strings.Contains(strings.TrimPrefix(a.URL, prefix), "/") {
			return fmt.Errorf("%s URL %q is not directly under %s", platform, a.URL, prefix)
		}
		file := filepath.Join(*dir, path.Base(a.URL))
		digest, err := fileDigest(file)
		if err != nil {
			return err
		}
		if err := updatesig.Verify(pub, m.Version, platform, digest, a.Signature); err != nil {
			return err
		}
		fmt.Printf("%s: %s verified against %s\n", platform, file, publicKeyPath)
	}
	if len(m.Platforms) != len(updatesig.Platforms) {
		return fmt.Errorf("latest.json has %d entries, expected %d", len(m.Platforms), len(updatesig.Platforms))
	}
	return nil
}

func fileDigest(name string) ([32]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	return updatesig.Digest(f)
}
