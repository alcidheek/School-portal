package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

// MediaOperations is implemented by the existing media service.
type MediaOperations interface {
	UploadSchoolLogo(context.Context, io.Reader) error
	DownloadSchoolLogo(context.Context) (io.ReadCloser, error)
	DeleteSchoolLogo(context.Context) error
	UploadProfilePicture(context.Context, uuid.UUID, io.Reader) error
	DownloadProfilePicture(context.Context, uuid.UUID) (io.ReadCloser, error)
	DeleteProfilePicture(context.Context, uuid.UUID) error
}

type MediaHandler struct{ media MediaOperations }

func NewMediaHandler(media MediaOperations) *MediaHandler { return &MediaHandler{media: media} }

// Allow multipart headers/boundaries in addition to the service's 2 MiB file cap.
const maxMediaRequestBytes = service.MaxMediaBytes + 64*1024

func (h *MediaHandler) UploadSchoolLogo(w http.ResponseWriter, r *http.Request) {
	if _, ok := authenticatedUserID(w, r); !ok {
		return
	}
	h.upload(w, r, func(input io.Reader) error { return h.media.UploadSchoolLogo(r.Context(), input) }, "/api/v1/school-profile/logo")
}
func (h *MediaHandler) UploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	h.upload(w, r, func(input io.Reader) error { return h.media.UploadProfilePicture(r.Context(), id, input) }, "/api/v1/users/me/profile-picture")
}
func (h *MediaHandler) DeleteSchoolLogo(w http.ResponseWriter, r *http.Request) {
	if _, ok := authenticatedUserID(w, r); !ok {
		return
	}
	if err := h.media.DeleteSchoolLogo(r.Context()); err != nil {
		writeMediaError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *MediaHandler) DeleteProfilePicture(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	if err := h.media.DeleteProfilePicture(r.Context(), id); err != nil {
		writeMediaError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *MediaHandler) DownloadSchoolLogo(w http.ResponseWriter, r *http.Request) {
	if _, ok := authenticatedUserID(w, r); !ok {
		return
	}
	h.download(w, r, func() (io.ReadCloser, error) { return h.media.DownloadSchoolLogo(r.Context()) })
}
func (h *MediaHandler) DownloadProfilePicture(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	h.download(w, r, func() (io.ReadCloser, error) { return h.media.DownloadProfilePicture(r.Context(), id) })
}

func (h *MediaHandler) upload(w http.ResponseWriter, r *http.Request, save func(io.Reader) error, url string) {
	if err := r.Context().Err(); err != nil {
		writeMediaError(w, err)
		return
	}
	if r.ContentLength > maxMediaRequestBytes {
		httpresponse.Error(w, "upload exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaRequestBytes)
	defer r.Body.Close()
	if err := r.ParseMultipartForm(maxMediaRequestBytes); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if err := r.Context().Err(); err != nil {
			writeMediaError(w, err)
			return
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpresponse.Error(w, "upload exceeds size limit", http.StatusRequestEntityTooLarge)
		} else {
			httpresponse.Error(w, "multipart/form-data with one file field is required", http.StatusBadRequest)
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["file"]
	if len(files) != 1 || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.Value) != 0 {
		httpresponse.Error(w, "exactly one file field is required", http.StatusBadRequest)
		return
	}
	if files[0].Size > service.MaxMediaBytes {
		httpresponse.Error(w, "image exceeds 2 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeMediaError(w, err)
		return
	}
	defer file.Close()
	if err := save(file); err != nil {
		writeMediaError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Message string `json:"message"`
		URL     string `json:"url"`
	}{"Image updated", url})
}

func (h *MediaHandler) download(w http.ResponseWriter, r *http.Request, load func() (io.ReadCloser, error)) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	reader, err := load()
	if err != nil {
		writeMediaError(w, err)
		return
	}
	defer reader.Close()
	buffered := bufio.NewReaderSize(mediaRequestReader{r.Context(), reader}, 32*1024)
	header, err := buffered.Peek(512)
	if err != nil && err != io.EOF {
		writeMediaError(w, err)
		return
	}
	mime := http.DetectContentType(header)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		writeMediaError(w, service.ErrMediaStorage)
		return
	}
	w.Header().Set("Content-Type", mime)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, err := io.Copy(w, buffered); err != nil {
		// Headers may already be sent. Abort rather than append JSON to an image
		// or allow a truncated image to appear as a completed response.
		if r.Context().Err() == nil {
			log.Print("media response stream failed")
		}
		panic(http.ErrAbortHandler)
	}
}

type mediaRequestReader struct {
	ctx   context.Context
	input io.Reader
}

func (r mediaRequestReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}

// Never echo error strings: even joined domain errors can contain cleanup details.
func writeMediaError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "Internal server error"
	switch {
	case errors.Is(err, service.ErrMediaCleanup):
		log.Print("media operation requires cleanup reconciliation")
	case errors.Is(err, service.ErrInvalidInput):
		status, message = http.StatusBadRequest, "Invalid image; supply a PNG, JPEG or WebP image within the size and pixel limits"
	case errors.Is(err, service.ErrNotFound):
		status, message = http.StatusNotFound, "Image not found"
	case errors.Is(err, service.ErrConflict):
		status, message = http.StatusConflict, "Image changed; refresh and try again"
	case errors.Is(err, context.Canceled):
		status, message = http.StatusRequestTimeout, "Request canceled"
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	httpresponse.Error(w, message, status)
}
