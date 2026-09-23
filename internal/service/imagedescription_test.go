package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"snzstudio/internal/config"
)

// Leading bytes enough for http.DetectContentType to classify each format.
var (
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	jpegBytes = append([]byte("\xff\xd8\xff\xe0"), make([]byte, 32)...)
	webpBytes = append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	gifBytes  = append([]byte("GIF89a"), make([]byte, 32)...)
	svgBytes  = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	heicBytes = append([]byte("\x00\x00\x00\x18ftypheic"), make([]byte, 32)...)
)

func imageDescriptionConfig(baseURL, model string) *config.Config {
	return testConfig(config.Settings{
		Editable: config.Editable{
			LLMBaseURL:              "http://127.0.0.1:1/v1",
			ImageDescriptionBaseURL: baseURL,
			ImageDescriptionModel:   model,
		},
		ImageDescriptionTimeoutMs: 2000,
	})
}

type capturedVisionRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Content []struct {
			Type     string `json:"type"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url"`
		} `json:"content"`
	} `json:"messages"`
}

func (r capturedVisionRequest) imageURL() string {
	for _, message := range r.Messages {
		for _, part := range message.Content {
			if part.Type == "image_url" && part.ImageURL != nil {
				return part.ImageURL.URL
			}
		}
	}
	return ""
}

func visionServer(t *testing.T, status int, reply string, captured *capturedVisionRequest) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if captured != nil {
			if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDescribeImageDisabledWithoutModel(t *testing.T) {
	svc := NewImageDescriptionService(imageDescriptionConfig("", ""))
	if svc.Enabled() {
		t.Fatal("Enabled() = true with no model")
	}
	if _, err := svc.DescribeImage(context.Background(), pngBytes); !errors.Is(err, ErrImageDescriptionDisabled) {
		t.Fatalf("err = %v, want ErrImageDescriptionDisabled", err)
	}
}

func TestDescribeImageRejectsFormatsOutsideAllowlist(t *testing.T) {
	svc := NewImageDescriptionService(imageDescriptionConfig("http://127.0.0.1:1/v1", "vision"))
	cases := map[string][]byte{
		"svg":   svgBytes,
		"heic":  heicBytes,
		"gif":   gifBytes,
		"empty": {},
		"text":  []byte("not an image at all"),
	}
	for name, image := range cases {
		if _, err := svc.DescribeImage(context.Background(), image); !errors.Is(err, ErrUnsupportedImageFormat) {
			t.Errorf("%s: err = %v, want ErrUnsupportedImageFormat", name, err)
		}
	}
}

func TestDescribeImageRejectsOversizeBeforeSending(t *testing.T) {
	svc := NewImageDescriptionService(imageDescriptionConfig("http://127.0.0.1:1/v1", "vision"))
	oversize := append(append([]byte(nil), pngBytes...), make([]byte, MaxImageDescriptionBytes)...)
	if _, err := svc.DescribeImage(context.Background(), oversize); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("err = %v, want ErrImageTooLarge", err)
	}
}

func TestDescribeImageSniffsFormatIntoDataURL(t *testing.T) {
	cases := map[string][]byte{"image/png": pngBytes, "image/jpeg": jpegBytes, "image/webp": webpBytes}
	for mimeType, image := range cases {
		var captured capturedVisionRequest
		server := visionServer(t, http.StatusOK, `{"choices":[{"message":{"content":"  灯台の地図  "}}]}`, &captured)
		svc := NewImageDescriptionService(imageDescriptionConfig(server.URL+"/v1/", "vision"))

		got, err := svc.DescribeImage(context.Background(), image)
		if err != nil {
			t.Fatalf("%s: DescribeImage: %v", mimeType, err)
		}
		if got != "灯台の地図" {
			t.Errorf("%s: description = %q, want trimmed content", mimeType, got)
		}
		if captured.Model != "vision" {
			t.Errorf("%s: model = %q, want vision", mimeType, captured.Model)
		}
		if url := captured.imageURL(); !strings.HasPrefix(url, "data:"+mimeType+";base64,") {
			t.Errorf("%s: image_url = %.40q, want a %s data URL", mimeType, url, mimeType)
		}
	}
}

func TestDescribeImageFallsBackToLLMBaseURL(t *testing.T) {
	server := visionServer(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`, nil)
	cfg := testConfig(config.Settings{
		Editable:                  config.Editable{LLMBaseURL: server.URL + "/v1", ImageDescriptionModel: "vision"},
		ImageDescriptionTimeoutMs: 2000,
	})
	if _, err := NewImageDescriptionService(cfg).DescribeImage(context.Background(), pngBytes); err != nil {
		t.Fatalf("DescribeImage with empty base URL: %v", err)
	}
}

func TestDescribeImageReportsEndpointFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		reply   string
		wantErr error
		wantMsg string
	}{
		{"model without vision", http.StatusBadRequest, `{"error":{"message":"model does not support image inputs."}}`, ErrImageDescriptionEndpoint, "does not support image inputs"},
		{"lm studio string error", http.StatusBadRequest, `{"error":"terminated"}`, ErrImageDescriptionEndpoint, "terminated"},
		{"error body with 200", http.StatusOK, `{"error":"Unexpected endpoint"}`, ErrImageDescriptionEndpoint, "Unexpected endpoint"},
		{"not json", http.StatusOK, `<html>`, ErrImageDescriptionEndpoint, "invalid response"},
		{"empty content", http.StatusOK, `{"choices":[{"message":{"content":"   "}}]}`, ErrImageDescriptionEmptyText, ""},
	}
	for _, tc := range cases {
		server := visionServer(t, tc.status, tc.reply, nil)
		svc := NewImageDescriptionService(imageDescriptionConfig(server.URL+"/v1", "vision"))
		_, err := svc.DescribeImage(context.Background(), pngBytes)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
			continue
		}
		if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
			t.Errorf("%s: err = %q, want it to carry %q", tc.name, err, tc.wantMsg)
		}
	}
}

func TestDescribeImageTimesOut(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })

	cfg := testConfig(config.Settings{
		Editable:                  config.Editable{ImageDescriptionBaseURL: server.URL + "/v1", ImageDescriptionModel: "vision"},
		ImageDescriptionTimeoutMs: 50,
	})
	start := time.Now()
	_, err := NewImageDescriptionService(cfg).DescribeImage(context.Background(), pngBytes)
	if !errors.Is(err, ErrImageDescriptionEndpoint) || !strings.Contains(err.Error(), "timed out after 50 ms") {
		t.Fatalf("err = %v, want an endpoint timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timed out after %v, want roughly the 50 ms budget", elapsed)
	}
}
