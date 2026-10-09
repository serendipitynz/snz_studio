package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"snzstudio/internal/updatesig"
)

func TestParseVersion(t *testing.T) {
	good := map[string]version{"0.1.0": {0, 1, 0}, "10.20.300": {10, 20, 300}}
	for s, want := range good {
		got, ok := parseVersion(s)
		if !ok || got != want {
			t.Errorf("parseVersion(%q) = %v, %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "0.1", "0.1.0.0", "v0.1.0", "0.1.0-rc1", "0.1.+1", "0..1", "0.1.x"} {
		if _, ok := parseVersion(s); ok {
			t.Errorf("parseVersion(%q) accepted", s)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.1", "0.1.0", 1},
		{"0.2.0", "0.10.0", -1}, // numeric, not lexical
		{"1.0.0", "0.99.99", 1},
	}
	for _, c := range cases {
		a, _ := parseVersion(c.a)
		b, _ := parseVersion(c.b)
		if got := compareVersions(a, b); got != c.want {
			t.Errorf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNewestVersionTagSkipsNonVersionDraftAndPrerelease(t *testing.T) {
	releases := []githubRelease{
		{TagName: "ruri-v3-30m-q8_0-2a6cb2d9"},
		{TagName: "v0.9.0", Draft: true},
		{TagName: "v0.8.0", Prerelease: true},
		{TagName: "v0.10.0-rc1"},
		{TagName: "v0.2.0"},
		{TagName: "v0.10.0"},
		{TagName: "v0.3.0"},
	}
	tag, _, ok := newestVersionTag(releases)
	if !ok || tag != "v0.10.0" {
		t.Fatalf("got %q, %v; want v0.10.0", tag, ok)
	}
	if _, _, ok := newestVersionTag(releases[:4]); ok {
		t.Fatal("picked a tag when every release is a model, draft, prerelease or malformed")
	}
}

func TestReleaseForChecksVersionAndPlatform(t *testing.T) {
	asset := updatesig.Asset{URL: "https://example.invalid/f", Signature: "c2ln"}
	m := updatesig.Manifest{Version: "0.2.0", Platforms: map[string]updatesig.Asset{"darwin-universal": asset}}
	if r, err := releaseFor("v0.2.0", m, "darwin-universal"); err != nil || r.Version != "0.2.0" || r.Asset != asset {
		t.Fatalf("got %+v, %v", r, err)
	}
	if _, err := releaseFor("v0.3.0", m, "darwin-universal"); err == nil {
		t.Error("accepted latest.json whose version differs from its tag")
	}
	if _, err := releaseFor("v0.2.0", m, "windows-amd64"); err == nil {
		t.Error("accepted latest.json without this platform's file")
	}
}

// fakeSource serves a release list, latest.json and one update file the way GitHub
// lays them out, signed with a key the test owns.
type fakeSource struct {
	t        *testing.T
	priv     ed25519.PrivateKey
	releases []githubRelease
	files    map[string][]byte // path -> body
}

func newFakeSource(t *testing.T) (*fakeSource, ed25519.PublicKey) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeSource{t: t, priv: priv, files: map[string][]byte{}}, pub
}

// publish adds a release whose latest.json declares manifestVersion and whose
// update file for this platform is body, signed for signedVersion.
func (f *fakeSource) publish(srv *httptest.Server, tag, manifestVersion, signedVersion string, body []byte) {
	f.releases = append(f.releases, githubRelease{TagName: tag})
	digest := sha256.Sum256(body)
	sig, err := updatesig.Sign(f.priv, signedVersion, Platform(), digest)
	if err != nil {
		f.t.Fatal(err)
	}
	filePath := "/releases/download/" + tag + "/update-file"
	f.files[filePath] = body
	m := updatesig.Manifest{Version: manifestVersion, Platforms: map[string]updatesig.Asset{
		Platform(): {URL: srv.URL + filePath, Signature: sig},
	}}
	f.files["/releases/download/"+tag+"/latest.json"], _ = json.Marshal(m)
}

func (f *fakeSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("User-Agent") == "" {
		http.Error(w, "no user agent", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/releases" {
		json.NewEncoder(w).Encode(f.releases)
		return
	}
	body, ok := f.files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(body)
}

func startFake(t *testing.T, current string) (*fakeSource, *httptest.Server, *Updater) {
	if Platform() == "" {
		t.Skip("no update platform for this OS")
	}
	src, pub := newFakeSource(t)
	srv := httptest.NewServer(src)
	t.Cleanup(srv.Close)
	return src, srv, &Updater{Client: srv.Client(), APIBase: srv.URL, DownloadBase: srv.URL, PublicKey: pub, Current: current}
}

func TestCheckAndDownload(t *testing.T) {
	src, srv, u := startFake(t, "0.1.0")
	body := []byte(strings.Repeat("update file ", 1000))
	src.publish(srv, "v0.2.0", "0.2.0", "0.2.0", body)
	src.releases = append(src.releases, githubRelease{TagName: "ruri-v3-30m-q8_0-2a6cb2d9"})

	r, err := u.Check(context.Background())
	if err != nil || r == nil || r.Tag != "v0.2.0" {
		t.Fatalf("Check = %+v, %v", r, err)
	}
	var last int64
	path, err := u.Download(context.Background(), r, t.TempDir(), func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(body) || last != int64(len(body)) {
		t.Fatalf("downloaded %d bytes, last progress %d, want %d", len(got), last, len(body))
	}
}

func TestCheckReportsNoUpdateAtOrAboveCurrent(t *testing.T) {
	for _, current := range []string{"0.2.0", "0.3.0"} {
		src, srv, u := startFake(t, current)
		src.publish(srv, "v0.2.0", "0.2.0", "0.2.0", []byte("x"))
		if r, err := u.Check(context.Background()); err != nil || r != nil {
			t.Errorf("current %s: Check = %+v, %v; want no update", current, r, err)
		}
	}
}

func TestCheckFailsWithoutAUsableRunningVersion(t *testing.T) {
	_, _, u := startFake(t, "")
	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("an unstamped build reported a result")
	}
}

func TestCheckRejectsManifestVersionMismatch(t *testing.T) {
	src, srv, u := startFake(t, "0.1.0")
	// An older, validly signed file listed under a newer tag.
	src.publish(srv, "v0.3.0", "0.2.0", "0.2.0", []byte("old file"))
	if r, err := u.Check(context.Background()); err == nil {
		t.Fatalf("accepted %+v", r)
	}
}

func TestDownloadRejectsBadSignatureAndLeavesNothing(t *testing.T) {
	cases := map[string]func(src *fakeSource, srv *httptest.Server){
		"signed for another version": func(src *fakeSource, srv *httptest.Server) {
			src.publish(srv, "v0.2.0", "0.2.0", "0.1.5", []byte("file"))
		},
		"file replaced after signing": func(src *fakeSource, srv *httptest.Server) {
			src.publish(srv, "v0.2.0", "0.2.0", "0.2.0", []byte("file"))
			src.files["/releases/download/v0.2.0/update-file"] = []byte("tampered")
		},
		"signed by another key": func(src *fakeSource, srv *httptest.Server) {
			_, src.priv, _ = ed25519.GenerateKey(nil)
			src.publish(srv, "v0.2.0", "0.2.0", "0.2.0", []byte("file"))
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			src, srv, u := startFake(t, "0.1.0")
			setup(src, srv)
			r, err := u.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if _, err := u.Download(context.Background(), r, dir, nil); err == nil {
				t.Fatal("verification passed")
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("left %d file(s) behind", len(entries))
			}
		})
	}
}

func TestNewUsesCompiledKeyAndBaseOverride(t *testing.T) {
	t.Setenv("SNZ_UPDATE_BASE_URL", "http://127.0.0.1:9/")
	u, err := New("0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	key, _ := updatesig.PublicKey()
	if !u.PublicKey.Equal(key) || u.APIBase != "http://127.0.0.1:9" || u.DownloadBase != u.APIBase {
		t.Fatalf("got %+v", u)
	}
}
