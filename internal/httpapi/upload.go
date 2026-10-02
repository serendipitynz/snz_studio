package httpapi

import (
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

// maxDocumentUploadBytes caps one document upload, image and text alike. It is
// not a security boundary — the disk and memory it protects are the user's own —
// but a guard against a stray drop of a huge file being parsed into memory and
// stored. 20MB keeps an original photo from a phone or camera storable (the UI
// downscales only the copy it sends for description, not the stored image), and
// is far beyond any text that is useful as chat context. One value for both keeps
// the limit simple to state.
const maxDocumentUploadBytes = 20 << 20

// documentUploadMultipartSlack covers the multipart framing plus the text fields
// (title, note, tags, derivedText) that ride beside the file in the same body.
const documentUploadMultipartSlack = 1 << 20

const documentUploadLimit = maxDocumentUploadBytes + documentUploadMultipartSlack

const unsupportedImageUploadMessage = "image must be PNG, JPEG, GIF or WebP"

// storedImageExtensions maps each image format a document may store to the
// extension its file is saved under. It is an allowlist judged from the bytes by
// http.DetectContentType, never from the client's filename or Content-Type:
// those are what let an .html or .svg upload be served back as a page on the API
// origin, where a script could read the token from its own URL.
var storedImageExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// inlineFileExtensions are the extensions /files serves for display. Anything
// else (an upload stored before the allowlist above existed) is only offered as
// a download. .jpeg is here for those earlier uploads; new ones are saved as .jpg.
var inlineFileExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

// sniffStoredImage returns the MIME type and extension of an allowlisted image,
// judged from its leading bytes, and rewinds file for the save that follows.
func sniffStoredImage(file multipart.File) (mimeType, ext string, ok bool, err error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", "", false, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", false, err
	}
	mimeType = http.DetectContentType(head[:n])
	ext, ok = storedImageExtensions[mimeType]
	return mimeType, ext, ok, nil
}

// setFileResponseHeaders keeps an uploaded file from running as a page on the
// API origin even when opened directly: nosniff stops the browser from guessing
// HTML out of it, the CSP blocks any script or subresource it carries, and a
// non-image is downloaded rather than rendered.
func setFileResponseHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if !inlineFileExtensions[strings.ToLower(filepath.Ext(name))] {
		w.Header().Set("Content-Disposition", "attachment")
	}
}
