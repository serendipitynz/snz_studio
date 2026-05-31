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
