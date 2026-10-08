package updatesig

import (
	"crypto/ed25519"
	"strings"
	"testing"
)

func TestCompiledPublicKeyParses(t *testing.T) {
	if _, err := PublicKey(); err != nil {
		t.Fatal(err)
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := Digest(strings.NewReader("update file"))
	sig, err := Sign(priv, "0.2.0", PlatformDarwinUniversal, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, "0.2.0", PlatformDarwinUniversal, digest, sig); err != nil {
		t.Fatalf("own signature rejected: %v", err)
	}

	other, _ := Digest(strings.NewReader("tampered file"))
	otherPub, _, _ := ed25519.GenerateKey(nil)
	cases := map[string]func() error{
		"another file":     func() error { return Verify(pub, "0.2.0", PlatformDarwinUniversal, other, sig) },
		"another version":  func() error { return Verify(pub, "0.3.0", PlatformDarwinUniversal, digest, sig) },
		"another platform": func() error { return Verify(pub, "0.2.0", PlatformWindowsAMD64, digest, sig) },
		"another key":      func() error { return Verify(otherPub, "0.2.0", PlatformDarwinUniversal, digest, sig) },
		"not base64":       func() error { return Verify(pub, "0.2.0", PlatformDarwinUniversal, digest, "!") },
	}
	for name, check := range cases {
		if check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMessageRejectsLineBreaks(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	var digest [32]byte
	if _, err := Sign(priv, "0.2.0\nplatform=windows-amd64", PlatformDarwinUniversal, digest); err == nil {
		t.Fatal("a version carrying a line break was signed")
	}
}

func TestPrivateKeyRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	got, err := ParsePrivateKey(EncodePrivateKey(priv) + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if !pub.Equal(got.Public()) {
		t.Fatal("decoded key derives another public key")
	}
	if _, err := ParsePrivateKey(EncodePublicKey(pub)[:10]); err == nil {
		t.Fatal("a truncated key was accepted")
	}
}
