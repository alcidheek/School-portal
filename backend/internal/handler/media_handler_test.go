package handler

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type mediaHTTPFake struct {
	err    error
	data   []byte
	id     uuid.UUID
	calls  int
	closed bool
	stream io.ReadCloser
}

func (m *mediaHTTPFake) UploadSchoolLogo(ctx context.Context, r io.Reader) error {
	m.calls++
	_, _ = io.ReadAll(r)
	return m.err
}
func (m *mediaHTTPFake) UploadProfilePicture(ctx context.Context, id uuid.UUID, r io.Reader) error {
	m.id = id
	return m.UploadSchoolLogo(ctx, r)
}
func (m *mediaHTTPFake) DeleteSchoolLogo(ctx context.Context) error { m.calls++; return m.err }
func (m *mediaHTTPFake) DeleteProfilePicture(ctx context.Context, id uuid.UUID) error {
	m.id = id
	return m.DeleteSchoolLogo(ctx)
}
func (m *mediaHTTPFake) DownloadSchoolLogo(ctx context.Context) (io.ReadCloser, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	if m.stream != nil {
		return m.stream, nil
	}
	return &mediaHTTPReader{Reader: bytes.NewReader(m.data), closed: &m.closed}, nil
}

type mediaFailingStream struct {
	data   []byte
	closed bool
}

func (s *mediaFailingStream) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, errors.New("private storage URL and secret")
	}
	n := copy(p, s.data)
	s.data = s.data[n:]
	return n, nil
}
func (s *mediaFailingStream) Close() error { s.closed = true; return nil }

func TestMediaHTTPStreamFailureAndCancellation(t *testing.T) {
	data := append(mediaPNG(t), make([]byte, 1024)...)
	stream := &mediaFailingStream{data: data}
	m := &mediaHTTPFake{stream: stream}
	h := NewMediaHandler(m)
	w := httptest.NewRecorder()
	func() {
		defer func() {
			if value := recover(); value != http.ErrAbortHandler {
				t.Fatalf("expected aborted response, got %v", value)
			}
		}()
		h.DownloadSchoolLogo(w, mediaIdentity(httptest.NewRequest("GET", "/", nil), uuid.New()))
	}()
	if !stream.closed || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), `"error"`) {
		t.Fatal("unsafe stream failure")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := mediaIdentity(mediaMultipart(t, mediaPNG(t), "file"), uuid.New())
	r = r.WithContext(context.WithValue(ctx, middleware.UserIDKey, uuid.NewString()))
	w = httptest.NewRecorder()
	before := m.calls
	h.UploadSchoolLogo(w, r)
	if w.Code != 408 || m.calls != before {
		t.Fatal("canceled upload reached service")
	}
	m.stream = nil
	m.data = mediaPNG(t)
	r = mediaIdentity(httptest.NewRequest("GET", "/", nil), uuid.New())
	r = r.WithContext(context.WithValue(ctx, middleware.UserIDKey, uuid.NewString()))
	w = httptest.NewRecorder()
	h.DownloadSchoolLogo(w, r)
	if w.Code != 408 || !m.closed {
		t.Fatal("canceled stream not closed")
	}
}
func (m *mediaHTTPFake) DownloadProfilePicture(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	m.id = id
	return m.DownloadSchoolLogo(ctx)
}

type mediaHTTPReader struct {
	*bytes.Reader
	closed *bool
}

func (r *mediaHTTPReader) Close() error { *r.closed = true; return nil }

func mediaPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func mediaMultipart(t *testing.T, data []byte, field string) *http.Request {
	t.Helper()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	if field != "" {
		part, err := writer.CreateFormFile(field, "untrusted.svg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("PUT", "/", &b)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}
func mediaIdentity(r *http.Request, id uuid.UUID) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, id.String()))
}

func TestMediaHTTPUploadsAndOwnership(t *testing.T) {
	for _, picture := range []bool{false, true} {
		m := &mediaHTTPFake{}
		h := NewMediaHandler(m)
		id := uuid.New()
		r := mediaIdentity(mediaMultipart(t, mediaPNG(t), "file"), id)
		r.URL.RawQuery = "user_id=" + uuid.NewString()
		w := httptest.NewRecorder()
		if picture {
			h.UploadProfilePicture(w, r)
		} else {
			h.UploadSchoolLogo(w, r)
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"url":"/api/v1/`) || m.calls != 1 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if picture && m.id != id {
			t.Fatal("query string changed ownership")
		}
	}
}

func TestMediaHTTPUploadLimits(t *testing.T) {
	for _, name := range []string{"missing", "wrong_field", "large_file", "large_body", "chunked_large_body", "not_multipart", "extra_owner", "duplicate_file"} {
		t.Run(name, func(t *testing.T) {
			m := &mediaHTTPFake{}
			h := NewMediaHandler(m)
			r := mediaMultipart(t, mediaPNG(t), "file")
			want := 400
			switch name {
			case "missing":
				r = mediaMultipart(t, nil, "")
			case "wrong_field":
				r = mediaMultipart(t, mediaPNG(t), "image")
			case "large_file":
				r = mediaMultipart(t, make([]byte, service.MaxMediaBytes+1), "file")
				want = 413
			case "large_body", "chunked_large_body":
				r = mediaMultipart(t, make([]byte, maxMediaRequestBytes+1), "file")
				want = 413
				if name == "chunked_large_body" {
					r.ContentLength = -1
				}
			case "not_multipart":
				r = httptest.NewRequest("PUT", "/", strings.NewReader("{}"))
				r.Header.Set("Content-Type", "application/json")
			case "extra_owner", "duplicate_file":
				var b bytes.Buffer
				mw := multipart.NewWriter(&b)
				part, _ := mw.CreateFormFile("file", "a.png")
				part.Write(mediaPNG(t))
				if name == "extra_owner" {
					mw.WriteField("user_id", uuid.NewString())
				} else {
					part, _ = mw.CreateFormFile("file", "b.png")
					part.Write(mediaPNG(t))
				}
				mw.Close()
				r = httptest.NewRequest("PUT", "/", &b)
				r.Header.Set("Content-Type", mw.FormDataContentType())
			}
			w := httptest.NewRecorder()
			h.UploadProfilePicture(w, mediaIdentity(r, uuid.New()))
			if w.Code != want || m.calls != 0 {
				t.Fatalf("%d %s calls=%d", w.Code, w.Body, m.calls)
			}
		})
	}
	// Exercise the real service validator; invalid content fails before repositories.
	h := NewMediaHandler(service.NewMediaService(nil, repository.NewSchoolProfileRepository(nil), nil))
	for _, data := range [][]byte{[]byte("<svg/>"), []byte("MZ executable"), nil} {
		w := httptest.NewRecorder()
		h.UploadSchoolLogo(w, mediaIdentity(mediaMultipart(t, data, "file"), uuid.New()))
		if w.Code != 400 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
}

func TestMediaHTTPDownloadAndDeletion(t *testing.T) {
	for _, picture := range []bool{false, true} {
		m := &mediaHTTPFake{data: mediaPNG(t)}
		h := NewMediaHandler(m)
		id := uuid.New()
		r := mediaIdentity(httptest.NewRequest("GET", "/?user_id="+uuid.NewString(), nil), id)
		w := httptest.NewRecorder()
		if picture {
			h.DownloadProfilePicture(w, r)
		} else {
			h.DownloadSchoolLogo(w, r)
		}
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), m.data) || !m.closed || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("invalid image response %d %s", w.Code, w.Body)
		}
		if picture && m.id != id {
			t.Fatal("download ownership changed")
		}
		w = httptest.NewRecorder()
		if picture {
			h.DeleteProfilePicture(w, r)
		} else {
			h.DeleteSchoolLogo(w, r)
		}
		if w.Code != 204 || w.Body.Len() != 0 {
			t.Fatalf("delete: %d %s", w.Code, w.Body)
		}
		if picture && m.id != id {
			t.Fatal("delete ownership changed")
		}
	}
}

func TestMediaHTTPAuthenticationAndSafeErrors(t *testing.T) {
	m := &mediaHTTPFake{}
	h := NewMediaHandler(m)
	for _, fn := range []http.HandlerFunc{h.UploadSchoolLogo, h.DownloadSchoolLogo, h.DeleteSchoolLogo, h.UploadProfilePicture, h.DownloadProfilePicture, h.DeleteProfilePicture} {
		w := httptest.NewRecorder()
		fn(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 401 || m.calls != 0 {
			t.Fatalf("missing auth: %d", w.Code)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{service.ErrNotFound, 404}, {service.ErrConflict, 409}, {context.Canceled, 408}, {context.DeadlineExceeded, 504},
		{errors.New("secret school-logos/private"), 500}, {errors.Join(service.ErrInvalidInput, errors.New("secret")), 400},
		{errors.Join(service.ErrConflict, service.ErrMediaCleanup, errors.New("secret")), 500},
	} {
		m.err = tc.err
		w := httptest.NewRecorder()
		h.DeleteSchoolLogo(w, mediaIdentity(httptest.NewRequest("DELETE", "/", nil), uuid.New()))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("unsafe error: %d %s", w.Code, w.Body)
		}
	}
	m.err = service.ErrNotFound
	w := httptest.NewRecorder()
	h.DownloadSchoolLogo(w, mediaIdentity(httptest.NewRequest("GET", "/", nil), uuid.New()))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	m.err = nil
	m.data = []byte("<svg/>")
	w = httptest.NewRecorder()
	h.DownloadSchoolLogo(w, mediaIdentity(httptest.NewRequest("GET", "/", nil), uuid.New()))
	if w.Code != 500 || !m.closed {
		t.Fatal("invalid stored content accepted")
	}
}
