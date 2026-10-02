package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Leading bytes enough for http.DetectContentType to classify each format.
var (
	uploadPNG  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	uploadJPEG = append([]byte("\xff\xd8\xff\xe0"), make([]byte, 32)...)
	uploadGIF  = append([]byte("GIF89a"), make([]byte, 32)...)
	uploadWebP = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
)

func TestCreateImageDocumentTakesFormatFromContent(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()
	base := "/api/projects/" + createProject(t, h, "Images") + "/documents"

	cases := []struct {
		data     []byte
		wantMIME string
		wantExt  string
	}{
		{uploadPNG, "image/png", ".png"},
		{uploadJPEG, "image/jpeg", ".jpg"},
		{uploadGIF, "image/gif", ".gif"},
		{uploadWebP, "image/webp", ".webp"},
	}
	for _, tc := range cases {
		t.Run(tc.wantMIME, func(t *testing.T) {
			// The filename and declared type both lie; neither reaches what is stored.
			rec := doMultipart(t, h, base, map[string]string{"type": "image"}, "file", "pic.html", tc.data, "text/html")
			wantStatus(t, rec, http.StatusCreated)
			var doc struct {
				FilePath *string `json:"filePath"`
				MimeType *string `json:"mimeType"`
			}
			unmarshalField(t, decodeJSONMap(t, rec), "document", &doc)
			if doc.FilePath == nil || !strings.HasSuffix(*doc.FilePath, tc.wantExt) {
				t.Fatalf("filePath = %v, want suffix %s", doc.FilePath, tc.wantExt)
			}
			if doc.MimeType == nil || *doc.MimeType != tc.wantMIME {
				t.Fatalf("mimeType = %v, want %s", doc.MimeType, tc.wantMIME)
			}

			rec = doJSON(t, h, "GET", *doc.FilePath, nil)
			wantStatus(t, rec, http.StatusOK)
			if !bytes.Equal(rec.Body.Bytes(), tc.data) {
				t.Fatal("served bytes differ from the upload")
			}
			if got := rec.Header().Get("Content-Type"); got != tc.wantMIME {
				t.Fatalf("Content-Type = %q, want %q", got, tc.wantMIME)
			}
			if got := rec.Header().Get("Content-Disposition"); got != "" {
				t.Fatalf("an image must display inline, got Content-Disposition %q", got)
			}
			wantFileSafetyHeaders(t, rec)
		})
	}
}

func TestCreateImageDocumentRejectsNonImages(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()
	projectID := createProject(t, h, "Rejects")
	base := "/api/projects/" + projectID + "/documents"

	cases := []struct {
		name, fileName, contentType string
		data                        []byte
	}{
		{"html", "page.html", "image/png", []byte("<!DOCTYPE html><script>alert(1)</script>")},
		{"svg", "icon.svg", "image/svg+xml", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"text named png", "note.png", "image/png", []byte("just some text")},
		{"heic", "photo.heic", "image/heic", append([]byte("\x00\x00\x00\x18ftypheic"), make([]byte, 32)...)},
		{"empty", "empty.png", "image/png", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doMultipart(t, h, base, map[string]string{"type": "image"}, "file", tc.fileName, tc.data, tc.contentType)
			wantError(t, rec, http.StatusBadRequest, unsupportedImageUploadMessage)
		})
	}

	if entries, err := os.ReadDir(srv.uploadDir); err == nil && len(entries) > 0 {
		t.Fatalf("upload dir has %d entries after rejected uploads, want none", len(entries))
	}
	docs, err := srv.documents.ListByProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("rejected uploads created %d documents, want none", len(docs))
	}
}

func TestCreateDocumentRefusesOversizeUploads(t *testing.T) {
	srv := newTestServer(t)
	h := srv.Handler()
	projectID := createProject(t, h, "Oversize")
	base := "/api/projects/" + projectID + "/documents"

	oversizeImage := append(append([]byte(nil), uploadPNG...), make([]byte, maxDocumentUploadBytes+documentUploadMultipartSlack)...)
	oversizeText := bytes.Repeat([]byte("a"), maxDocumentUploadBytes+documentUploadMultipartSlack+1)

	cases := []struct {
		name, docType, fileName string
		data                    []byte
		declareLength           bool
	}{
		{"image with length", "image", "big.png", oversizeImage, true},
		{"image without length", "image", "big.png", oversizeImage, false},
		{"text with length", "text", "big.txt", oversizeText, true},
		{"text without length", "text", "big.txt", oversizeText, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, contentType := multipartBody(t, map[string]string{"type": tc.docType}, tc.fileName, tc.data)
			req := httptest.NewRequest("POST", base, nil)
			req.Header.Set("Content-Type", contentType)
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = -1
			if tc.declareLength {
				req.ContentLength = int64(len(body))
			}
			rec := serve(h, req)
			wantError(t, rec, http.StatusRequestEntityTooLarge, "upload is too large")
		})
	}

	if entries, err := os.ReadDir(srv.uploadDir); err == nil && len(entries) > 0 {
		t.Fatalf("upload dir has %d entries after refused uploads, want none", len(entries))
	}
	docs, err := srv.documents.ListByProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("refused uploads created %d documents, want none", len(docs))
	}
}

// An upload stored before the content check could be an .html or .svg; /files
// must offer it only as a download, never render it on the API origin.
func TestFilesServesNonImagesAsAttachments(t *testing.T) {
	srv := newTestServer(t)
	if err := os.MkdirAll(srv.uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"legacy.html", "legacy.svg", "legacy.txt"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(srv.uploadDir, name), []byte("<script>alert(1)</script>"), 0o644); err != nil {
				t.Fatal(err)
			}
			rec := doJSON(t, srv.Handler(), "GET", "/files/"+name, nil)
			wantStatus(t, rec, http.StatusOK)
			if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
				t.Fatalf("Content-Disposition = %q, want attachment", got)
			}
			wantFileSafetyHeaders(t, rec)
		})
	}
}

func wantFileSafetyHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Fatalf("Content-Security-Policy = %q, want default-src 'none'", got)
	}
}

func multipartBody(t *testing.T, fields map[string]string, fileName string, fileData []byte) ([]byte, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, fileName))
	part, err := mw.CreatePart(mh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(fileData); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), mw.FormDataContentType()
}
