package embed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// progressFunc receives cumulative bytes written and the expected total (0 if
// unknown). It is called frequently during a download for UI status.
type progressFunc func(downloaded, total int64)

// verifyFile reports whether path matches spec's size and sha256. A missing file or
// any read error is reported as not-verified (false) rather than an error, so the
// caller can simply (re)download.
func verifyFile(path string, spec ModelSpec) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if spec.SizeBytes > 0 && info.Size() != spec.SizeBytes {
		return false
	}
	if spec.SHA256 == "" {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == spec.SHA256
}

// seedBundledModel copies the model GGUF shipped inside the app bundle (packaged
// builds stage it next to the llama-server sidecar — see scripts/build-mac-signed.sh)
// into dir on first launch, so the network download is skipped entirely.
//
// It is best-effort: if no verified copy is present in the bundle (e.g. `wails dev`,
// an unbundled build, or a corrupt/missing file) it is a no-op and the caller falls
// through to downloadModel. The copy goes via a temp file and is verified+renamed
// atomically, so a verified dest is never left half-written.
func seedBundledModel(dir string, spec ModelSpec) {
	dest := filepath.Join(dir, spec.FileName)
	if verifyFile(dest, spec) {
		return // already present in the per-user models dir
	}
	src := bundledModelPath(spec.FileName)
	if src == "" || !verifyFile(src, spec) {
		return // no usable bundled copy; let downloadModel handle it
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp := dest + ".seed"
	if err := copyFile(src, tmp); err != nil || !verifyFile(tmp, spec) {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Rename(tmp, dest)
}

// copyFile copies src to dst, truncating dst if it exists.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// downloadModel ensures spec is present and verified under dir, returning its path.
// It is idempotent (returns immediately if the file already verifies), streams to a
// <file>.part with HTTP Range resume, verifies size+sha256, and atomically renames
// on success. A failed verification removes the .part so the next attempt restarts.
func downloadModel(ctx context.Context, client *http.Client, spec ModelSpec, dir string, progress progressFunc) (string, error) {
	dest := filepath.Join(dir, spec.FileName)
	if verifyFile(dest, spec) {
		return dest, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	part := dest + ".part"

	var startAt int64
	if info, err := os.Stat(part); err == nil {
		startAt = info.Size()
		if spec.SizeBytes > 0 && startAt >= spec.SizeBytes {
			// Stale/oversized leftover: start over.
			_ = os.Remove(part)
			startAt = 0
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return "", err
	}
	if startAt > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startAt))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	flag := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusOK:
		// Server ignored Range (or none was sent): restart from the beginning.
		startAt = 0
		flag |= os.O_TRUNC
	case http.StatusPartialContent:
		flag |= os.O_APPEND
	default:
		return "", fmt.Errorf("download %s: unexpected status %d", spec.FileName, resp.StatusCode)
	}

	total := spec.SizeBytes
	if total == 0 && resp.ContentLength > 0 {
		total = startAt + resp.ContentLength
	}

	f, err := os.OpenFile(part, flag, 0o644)
	if err != nil {
		return "", err
	}
	written := startAt
	buf := make([]byte, 1<<20)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return "", werr
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	if !verifyFile(part, spec) {
		_ = os.Remove(part)
		return "", fmt.Errorf("download %s: size/checksum mismatch", spec.FileName)
	}
	if err := os.Rename(part, dest); err != nil {
		return "", err
	}
	return dest, nil
}
