package httpapi

import (
	"errors"
	"io"
	"net/http"

	"snzstudio/internal/service"
)

// imageDescriptionMultipartSlack covers the multipart framing (boundary lines and
// part headers) that rides on top of the image bytes in a describe request.
const imageDescriptionMultipartSlack = 64 << 10

// handleGetImageDescription tells the UI up front whether description can run and
// what it accepts, so the action is disabled with a reason rather than failing
// after upload.
func (s *Server) handleGetImageDescription(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":  s.imageDescription.Enabled(),
		"maxBytes": service.MaxImageDescriptionBytes,
		"formats":  service.ImageDescriptionFormats(),
	})
}

// handleDescribeImage returns a draft description of the uploaded image. Nothing
// is stored: the image is not written to the upload directory and no document is
// created.
func (s *Server) handleDescribeImage(w http.ResponseWriter, r *http.Request) {
	// Checked before reading anything, so an oversized upload is refused without
	// buffering it; MaxBytesReader covers a body that declares no length.
	limit := int64(service.MaxImageDescriptionBytes + imageDescriptionMultipartSlack)
	if r.ContentLength > limit {
		writeError(w, http.StatusRequestEntityTooLarge, service.ErrImageTooLarge.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	file, _, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, service.ErrImageTooLarge.Error())
		case errors.Is(err, http.ErrMissingFile):
			writeError(w, http.StatusBadRequest, "image file is required")
		default:
			writeError(w, http.StatusBadRequest, "invalid multipart form")
		}
		return
	}
	defer file.Close()
	image, err := io.ReadAll(file)
	if err != nil {
		fail(w, err)
		return
	}

	description, err := s.imageDescription.DescribeImage(r.Context(), image)
	if err != nil {
		status, message := imageDescriptionErrorResponse(err)
		writeError(w, status, message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"description": description})
}

func imageDescriptionErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrImageDescriptionDisabled):
		// The request is well formed; it is the configuration that rules it out.
		return http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrUnsupportedImageFormat):
		return http.StatusUnsupportedMediaType, err.Error()
	case errors.Is(err, service.ErrImageTooLarge):
		return http.StatusRequestEntityTooLarge, err.Error()
	case errors.Is(err, service.ErrImageDescriptionEndpoint), errors.Is(err, service.ErrImageDescriptionEmptyText):
		// The model endpoint failed, not this server or the request.
		return http.StatusBadGateway, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
