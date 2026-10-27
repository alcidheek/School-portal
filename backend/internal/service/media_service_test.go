/*
The embedded WebP fixture below is from golang.org/x/image/testdata.
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/
package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type mediaRepoFake struct {
	mu             sync.Mutex
	key            *string
	getErr, casErr error
	commitOnError  bool
	beforeCAS      func()
}

func copyKey(k *string) *string {
	if k == nil {
		return nil
	}
	v := *k
	return &v
}
func (r *mediaRepoFake) GetLogoObjectKey(ctx context.Context) (*string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return copyKey(r.key), r.getErr
}
func (r *mediaRepoFake) ReplaceLogoObjectKey(ctx context.Context, old, next *string) error {
	if r.beforeCAS != nil {
		r.beforeCAS()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if (old == nil) != (r.key == nil) || (old != nil && *old != *r.key) {
		return repository.ErrMediaObjectConflict
	}
	if r.casErr == nil || r.commitOnError {
		r.key = copyKey(next)
	}
	return r.casErr
}
func (r *mediaRepoFake) GetProfilePictureObjectKey(ctx context.Context, id uuid.UUID) (*string, error) {
	return r.GetLogoObjectKey(ctx)
}
func (r *mediaRepoFake) ReplaceProfilePictureObjectKey(ctx context.Context, id uuid.UUID, old, next *string) error {
	return r.ReplaceLogoObjectKey(ctx, old, next)
}

type mediaStorageFake struct {
	mu                                sync.Mutex
	objects                           map[string][]byte
	deleted                           []string
	uploadErr, deleteErr, downloadErr error
	mime                              string
}

func (s *mediaStorageFake) Upload(ctx context.Context, key string, input io.Reader, size int64, mime string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := io.ReadAll(input)
	s.objects[key] = data // Also model an upload that succeeds remotely but returns an error.
	s.mime = mime
	return s.uploadErr
}
func (s *mediaStorageFake) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(s.objects[key])), s.downloadErr
}
func (s *mediaStorageFake) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, key)
	delete(s.objects, key)
	return nil
}
func mediaFixture() (*MediaService, *mediaRepoFake, *mediaStorageFake) {
	r := &mediaRepoFake{}
	s := &mediaStorageFake{objects: make(map[string][]byte)}
	return NewMediaService(s, r, r), r, s
}
func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestMediaValidation(t *testing.T) {
	valid := pngFixture(t)
	// Lossless WebP fixture from golang.org/x/image testdata (Go BSD license).
	webp, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatal(err)
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"png", valid, true}, {"jpeg", jpg.Bytes(), true}, {"webp", webp, true},
		{"empty", nil, false}, {"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), false},
		{"exe", []byte("MZ executable"), false}, {"truncated", valid[:len(valid)-15], false},
		{"oversized", make([]byte, MaxMediaBytes+1), false},
		{"fake_webp", []byte("RIFF0000WEBPVP8 fake"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := validateMedia(context.Background(), bytes.NewReader(tc.data))
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := validateMedia(ctx, bytes.NewReader(valid)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMediaPayloadStrippingAndSizeBoundary(t *testing.T) {
	input := append(pngFixture(t), []byte("MZ appended executable payload")...)
	data, mime, err := validateMedia(context.Background(), bytes.NewReader(input))
	if err != nil || mime != "image/png" || bytes.Contains(data, []byte("executable payload")) {
		t.Fatalf("payload stripping failed: %v", err)
	}
	input = append(pngFixture(t), make([]byte, MaxMediaBytes-len(pngFixture(t)))...)
	if _, _, err := validateMedia(context.Background(), bytes.NewReader(input)); err != nil {
		t.Fatal("exact size boundary rejected", err)
	}
	if _, _, err := validateMedia(context.Background(), nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 4097, 4096))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateMedia(context.Background(), bytes.NewReader(b.Bytes())); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("excessive decoded size accepted", err)
	}
}

func TestMediaCancellationAfterUpload(t *testing.T) {
	s, r, store := mediaFixture()
	ctx, cancel := context.WithCancel(context.Background())
	r.beforeCAS = cancel
	r.casErr = context.Canceled
	err := s.UploadSchoolLogo(ctx, bytes.NewReader(pngFixture(t)))
	var pending *MediaCleanupError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &pending) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if len(store.objects) != 1 || r.key != nil {
		t.Fatal("unsafe canceled write handling")
	}
	if err := pending.Retry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.objects) != 0 {
		t.Fatal("canceled upload not cleaned")
	}
}

func TestMediaConcurrentDeleteAndReplacement(t *testing.T) {
	s, r, store := mediaFixture()
	old := "school-logos/old"
	r.key = &old
	store.objects[old] = []byte("old")
	var arrived sync.WaitGroup
	arrived.Add(2)
	r.beforeCAS = func() { arrived.Done(); arrived.Wait() }
	results := make(chan error, 2)
	input := pngFixture(t)
	go func() { results <- s.UploadSchoolLogo(context.Background(), bytes.NewReader(input)) }()
	go func() { results <- s.DeleteSchoolLogo(context.Background()) }()
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	if r.key != nil {
		if _, ok := store.objects[*r.key]; !ok {
			t.Fatal("referenced image deleted")
		}
	} else if len(store.objects) != 0 {
		t.Fatal("unreferenced upload remains")
	}
}

func TestMediaUploadReplaceDownloadDelete(t *testing.T) {
	for _, picture := range []bool{false, true} {
		s, r, store := mediaFixture()
		id := uuid.New()
		upload := func() error {
			if picture {
				return s.UploadProfilePicture(context.Background(), id, bytes.NewReader(pngFixture(t)))
			}
			return s.UploadSchoolLogo(context.Background(), bytes.NewReader(pngFixture(t)))
		}
		if err := upload(); err != nil {
			t.Fatal(err)
		}
		old := *r.key
		prefix := "school-logos/"
		if picture {
			prefix = "profile-pictures/" + id.String() + "/"
		}
		if !strings.HasPrefix(old, prefix) || store.mime != "image/png" {
			t.Fatal("incorrect key or MIME")
		}
		if _, err := uuid.Parse(strings.TrimPrefix(old, prefix)); err != nil {
			t.Fatal("key lacks random UUID")
		}
		if err := upload(); err != nil {
			t.Fatal(err)
		}
		if *r.key == old || len(store.deleted) != 1 || store.deleted[0] != old {
			t.Fatal("replacement did not clean previous object")
		}
		var reader io.ReadCloser
		var err error
		if picture {
			reader, err = s.DownloadProfilePicture(context.Background(), id)
		} else {
			reader, err = s.DownloadSchoolLogo(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(reader)
		reader.Close()
		if !bytes.Equal(data, pngFixture(t)) {
			t.Fatal("wrong downloaded image")
		}
		if picture {
			err = s.DeleteProfilePicture(context.Background(), id)
		} else {
			err = s.DeleteSchoolLogo(context.Background())
		}
		if err != nil || r.key != nil || len(store.objects) != 0 {
			t.Fatalf("delete failed: %v", err)
		}
	}
}

func TestMediaFailureCompensation(t *testing.T) {
	for _, tc := range []string{"upload", "database", "confirmed_database_failure", "conflict", "cleanup", "ambiguous_commit"} {
		t.Run(tc, func(t *testing.T) {
			s, r, store := mediaFixture()
			old := "school-logos/old"
			r.key = &old
			store.objects[old] = []byte("old")
			switch tc {
			case "upload":
				store.uploadErr = errors.New("private backend details")
			case "database":
				r.casErr = errors.New("private SQL details")
			case "confirmed_database_failure":
				r.casErr = &pq.Error{Code: "23514", Message: "private SQL details"}
			case "conflict":
				r.casErr = repository.ErrMediaObjectConflict
			case "cleanup":
				store.deleteErr = errors.New("private credentials")
			case "ambiguous_commit":
				r.casErr = errors.New("connection lost")
				r.commitOnError = true
			}
			err := s.UploadSchoolLogo(context.Background(), bytes.NewReader(pngFixture(t)))
			if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), old) {
				t.Fatalf("expected safe error, got %v", err)
			}
			if _, ok := store.objects[*r.key]; !ok {
				t.Fatal("deleted referenced object")
			}
			if tc == "conflict" && !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if tc == "cleanup" || tc == "ambiguous_commit" || tc == "database" {
				var pending *MediaCleanupError
				if !errors.As(err, &pending) || !errors.Is(err, ErrMediaCleanup) {
					t.Fatal("missing cleanup retry")
				}
				if tc == "cleanup" {
					if !pending.ReferenceUpdated() {
						t.Fatal("committed flag missing")
					}
					store.deleteErr = nil
					if err := pending.Retry(context.Background()); err != nil {
						t.Fatal(err)
					}
				} else if tc == "database" {
					if err := pending.Retry(context.Background()); err != nil {
						t.Fatal(err)
					}
					if len(store.objects) != 1 {
						t.Fatal("rollback cleanup failed")
					}
				} else {
					if err := pending.Retry(context.Background()); err != nil {
						t.Fatal(err)
					}
					if _, ok := store.objects[*r.key]; !ok {
						t.Fatal("retry deleted referenced upload")
					}
					if len(store.objects) != 1 {
						t.Fatal("ambiguous commit did not reconcile previous image")
					}
				}
			} else if len(store.objects) != 1 || *r.key != old {
				t.Fatal("failed operation left unreferenced upload")
			}
		})
	}
}

func TestMediaConcurrentReplacement(t *testing.T) {
	s, r, store := mediaFixture()
	old := "school-logos/old"
	r.key = &old
	store.objects[old] = []byte("old")
	var arrived sync.WaitGroup
	arrived.Add(2)
	r.beforeCAS = func() { arrived.Done(); arrived.Wait() }
	results := make(chan error, 2)
	input := pngFixture(t)
	for i := 0; i < 2; i++ {
		go func() { results <- s.UploadSchoolLogo(context.Background(), bytes.NewReader(input)) }()
	}
	wins, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 || len(store.objects) != 1 {
		t.Fatalf("wins=%d conflicts=%d objects=%d", wins, conflicts, len(store.objects))
	}
	if _, ok := store.objects[*r.key]; !ok {
		t.Fatal("winner's image deleted")
	}
}

func TestMediaDeleteFailuresAndMissingResources(t *testing.T) {
	s, r, store := mediaFixture()
	if err := s.DeleteSchoolLogo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DownloadSchoolLogo(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	r.getErr = sql.ErrNoRows
	if err := s.UploadSchoolLogo(context.Background(), bytes.NewReader(pngFixture(t))); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	r.getErr = nil
	old := "school-logos/old"
	r.key = &old
	store.objects[old] = []byte("old")
	r.casErr = repository.ErrMediaObjectConflict
	if err := s.DeleteSchoolLogo(context.Background()); !errors.Is(err, ErrConflict) || len(store.deleted) != 0 {
		t.Fatal("conflict deleted referenced object")
	}
	r.casErr = nil
	store.deleteErr = errors.New("failure")
	err := s.DeleteSchoolLogo(context.Background())
	var pending *MediaCleanupError
	if !errors.As(err, &pending) || r.key != nil || !pending.ReferenceUpdated() {
		t.Fatal("cleanup failure not surfaced")
	}
	store.deleteErr = nil
	if err := pending.Retry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.UploadProfilePicture(context.Background(), uuid.Nil, bytes.NewReader(pngFixture(t))); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
}
