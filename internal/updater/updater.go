// Package updater finds a newer published version of the app, downloads its update
// file and verifies it against the public key compiled into this build
// (internal/updatesig). Putting the verified file in place is platform-specific;
// see install_darwin.go and install_windows.go.
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"snzstudio/internal/updatesig"
)

const (
	githubAPIBase      = "https://api.github.com/repos/serendipitynz/snz_studio"
	githubDownloadBase = "https://github.com/serendipitynz/snz_studio"

	// maxUpdateSize bounds a download whose server keeps sending. The macOS archive,
	// built-in model included, is about 64MB.
	maxUpdateSize = 1 << 30
)

// Updater checks one release source. Both bases name the same repository: APIBase
// serves the release list (/releases), DownloadBase the assets
// (/releases/download/<tag>/…) and the release pages.
type Updater struct {
	Client       *http.Client
	APIBase      string
	DownloadBase string
	PublicKey    ed25519.PublicKey
	// Current is the running version, "MAJOR.MINOR.PATCH" without the "v".
	Current string
}

// New returns an Updater for this repository's GitHub releases.
//
// SNZ_UPDATE_BASE_URL points both bases at one server, which is how an update is
// exercised end to end without publishing a release. It cannot widen what gets
// installed: every file is still verified against the compiled-in key.
func New(current string) (*Updater, error) {
	key, err := updatesig.PublicKey()
	if err != nil {
		return nil, err
	}
	u := &Updater{
		Client:       &http.Client{},
		APIBase:      githubAPIBase,
		DownloadBase: githubDownloadBase,
		PublicKey:    key,
		Current:      current,
	}
	if base := strings.TrimRight(os.Getenv("SNZ_UPDATE_BASE_URL"), "/"); base != "" {
		u.APIBase, u.DownloadBase = base, base
	}
	return u, nil
}

// Release is a newer version that carries an update file for this platform.
type Release struct {
	Version string
	Tag     string
	Asset   updatesig.Asset
}

// ReleasesPage is the list of all releases, where an update that cannot be
// installed in place is downloaded by hand.
func (u *Updater) ReleasesPage() string {
	return u.DownloadBase + "/releases"
}

// ReleasePage is one version's release page, which carries its notes.
func (u *Updater) ReleasePage(tag string) string {
	return u.DownloadBase + "/releases/tag/" + tag
}

// Platform is this build's key in latest.json, or "" where no update file exists.
func Platform() string {
	switch {
	case runtime.GOOS == "darwin":
		return updatesig.PlatformDarwinUniversal
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return updatesig.PlatformWindowsAMD64
	}
	return ""
}

// Check returns the newest release when it is newer than the running version, and
// nil when there is none.
//
// The newest release is picked from the release list rather than from
// releases/latest: model files are released under their own tags here, and
// publishing one moves "latest" onto it.
func (u *Updater) Check(ctx context.Context) (*Release, error) {
	current, ok := parseVersion(u.Current)
	if !ok {
		return nil, fmt.Errorf("updater: running version %q is not MAJOR.MINOR.PATCH", u.Current)
	}
	platform := Platform()
	if platform == "" {
		return nil, fmt.Errorf("updater: no update files are published for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	var releases []githubRelease
	if err := u.getJSON(ctx, u.APIBase+"/releases?per_page=100", &releases); err != nil {
		return nil, err
	}
	tag, newest, found := newestVersionTag(releases)
	if !found || compareVersions(newest, current) <= 0 {
		return nil, nil
	}

	var manifest updatesig.Manifest
	if err := u.getJSON(ctx, u.DownloadBase+"/releases/download/"+tag+"/latest.json", &manifest); err != nil {
		return nil, err
	}
	return releaseFor(tag, manifest, platform)
}

// releaseFor checks latest.json against the tag it was fetched from. The version
// also has to match because the signature covers the version latest.json declares:
// without this check, a validly signed older file could be listed under a newer tag.
func releaseFor(tag string, m updatesig.Manifest, platform string) (*Release, error) {
	version := strings.TrimPrefix(tag, "v")
	if m.Version != version {
		return nil, fmt.Errorf("updater: latest.json of %s declares version %q", tag, m.Version)
	}
	asset, ok := m.Platforms[platform]
	if !ok || asset.URL == "" || asset.Signature == "" {
		return nil, fmt.Errorf("updater: latest.json of %s has no %s file", tag, platform)
	}
	return &Release{Version: version, Tag: tag, Asset: asset}, nil
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// newestVersionTag picks the highest vMAJOR.MINOR.PATCH among published,
// non-prerelease releases. Other tags (the model releases) are skipped.
func newestVersionTag(releases []githubRelease) (string, version, bool) {
	var bestTag string
	var best version
	found := false
	for _, r := range releases {
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, "v") {
			continue
		}
		v, ok := parseVersion(r.TagName[1:])
		if !ok {
			continue
		}
		if !found || compareVersions(v, best) > 0 {
			bestTag, best, found = r.TagName, v, true
		}
	}
	return bestTag, best, found
}

type version [3]int

// parseVersion accepts exactly what release.yml allows a tag to carry after its
// "v": three dot-separated decimal numbers.
func parseVersion(s string) (version, bool) {
	var v version
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return v, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func compareVersions(a, b version) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func (u *Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// The GitHub API refuses requests without a User-Agent.
	req.Header.Set("User-Agent", "snz-studio/"+u.Current)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("updater: GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

func (u *Updater) getJSON(ctx context.Context, url string, v any) error {
	resp, err := u.get(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v); err != nil {
		return fmt.Errorf("updater: read %s: %w", url, err)
	}
	return nil
}

// Download saves the release's update file into dir and verifies its signature,
// returning the file's path. A file that fails verification is removed, so nothing
// unverified is left for a later step to pick up. progress may be nil.
func (u *Updater) Download(ctx context.Context, r *Release, dir string, progress func(done, total int64)) (string, error) {
	platform := Platform()
	resp, err := u.get(ctx, r.Asset.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	path := filepath.Join(dir, updateFileName(platform))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	w := &progressWriter{hash: h, total: max(resp.ContentLength, 0), report: progress}
	n, copyErr := io.Copy(io.MultiWriter(f, w), io.LimitReader(resp.Body, maxUpdateSize+1))
	w.finish()
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("updater: download %s: %w", r.Asset.URL, err)
	}
	if n > maxUpdateSize {
		os.Remove(path)
		return "", fmt.Errorf("updater: %s is larger than %d bytes", r.Asset.URL, maxUpdateSize)
	}

	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	if err := updatesig.Verify(u.PublicKey, r.Version, platform, digest, r.Asset.Signature); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func updateFileName(platform string) string {
	if platform == updatesig.PlatformWindowsAMD64 {
		return "snz-studio-update-installer.exe"
	}
	return "snz-studio-update.app.zip"
}

// progressWriter hashes what is written and reports the running total whenever it
// crosses another percent (or MiB, when the size is unknown), so a fast download
// does not flood the UI with events. total is 0 when the server sent no length.
type progressWriter struct {
	hash   hash.Hash
	total  int64
	done   int64
	last   int64
	report func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.hash.Write(b)
	p.done += int64(len(b))
	if p.report != nil {
		step := p.total / 100
		if step <= 0 {
			step = 1 << 20
		}
		if p.done-p.last >= step {
			p.last = p.done
			p.report(p.done, p.total)
		}
	}
	return len(b), nil
}

// finish reports the final count if the last write did not.
func (p *progressWriter) finish() {
	if p.report != nil && p.done != p.last {
		p.last = p.done
		p.report(p.done, p.total)
	}
}
