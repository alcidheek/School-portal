package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/config"
	"github.com/minio/minio-go/v7"
)

func TestMinIOInitialization(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code, want string
	}{
		{"available", 200, "", ""},
		{"invalid_credentials", 403, "InvalidAccessKeyId", "authentication failed"},
		{"missing_bucket", 404, "NoSuchBucket", "does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				if r.URL.Query().Has("location") {
					_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
					return
				}
				w.Header().Set("X-Minio-Error-Code", tc.code)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			client, err := NewMinIOStorage(context.Background(), strings.TrimPrefix(server.URL, "http://"), "test-app", "test-secret", "test-media", false)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestMinIOInitializationDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := NewMinIOStorage(ctx, strings.TrimPrefix(server.URL, "http://"), "test-app", "test-secret", "test-media", false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

// Opt in explicitly: ordinary tests never read the developer's .env or contact MinIO.
func TestLocalMinIOIntegration(t *testing.T) {
	if os.Getenv("TEST_LOCAL_MINIO") != "1" {
		t.Skip("local integration disabled")
	}
	t.Chdir("../..")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := NewMinIOStorage(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	_, err = NewMinIOStorage(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, "deliberately-invalid-test-secret", cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected authentication failure, got %v", err)
	}
	_, err = NewMinIOStorage(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, "blueprint-missing-test-bucket", cfg.MinIOUseSSL)
	// A restricted application user may receive AccessDenied for other buckets.
	if err == nil || (!strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "access denied")) {
		t.Fatalf("expected missing-bucket or permission error, got %v", err)
	}
}

func TestMinIOErrorsDoNotExposeServerDetails(t *testing.T) {
	for _, err := range []error{errors.New("private-secret"), minio.ErrorResponse{Code: "AccessDenied", Message: "private-secret"}} {
		if strings.Contains(safeStorageError("check MinIO bucket", err).Error(), "private-secret") {
			t.Fatal("server details exposed")
		}
	}
	if _, err := NewMinIOStorage(context.Background(), "", "test-app", "test-secret", "test-media", false); err == nil {
		t.Fatal("missing endpoint accepted")
	}
	if _, err := NewMinIOStorage(context.Background(), "http://private-secret@invalid", "test-app", "test-secret", "test-media", false); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("expected safe endpoint error")
	}
}
