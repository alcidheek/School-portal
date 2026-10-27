package config

import (
	"os"
	"strings"
	"testing"
)

func TestJWTSecretConfiguration(t *testing.T) {
	// Load never reads the developer's .env in this test.
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("# isolated test configuration\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "unused-test-database")
	setMinIOTestEnv(t)
	for _, secret := range []string{"", "short", strings.Repeat("x", 31), strings.Repeat("x", 32)} {
		t.Setenv("JWT_SECRET", secret)
		_, err := Load()
		if (err == nil) != (len(secret) >= 32) {
			t.Fatalf("secret length %d: %v", len(secret), err)
		}
		if err != nil && secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatal("secret exposed")
		}
	}
}

func setMinIOTestEnv(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"MINIO_ENDPOINT": "127.0.0.1:9000", "MINIO_BUCKET": "test-media",
		"MINIO_ACCESS_KEY": "test-app", "MINIO_SECRET_KEY": "test-secret",
		"MINIO_USE_SSL": "false",
	} {
		t.Setenv(name, value)
	}
}

func TestMinIOConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("# isolated configuration\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "unused-test-database")
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	setMinIOTestEnv(t)
	for _, value := range []string{"false", "true", "invalid-private-value", ""} {
		t.Run("ssl_"+value, func(t *testing.T) {
			t.Setenv("MINIO_USE_SSL", value)
			cfg, err := Load()
			if value == "true" || value == "false" {
				if err != nil {
					t.Fatal(err)
				}
				if cfg.MinIOUseSSL != (value == "true") || cfg.MinIOBucket != "test-media" || cfg.MinIOEndpoint != "127.0.0.1:9000" || cfg.MinIOAccessKey != "test-app" || cfg.MinIOSecretKey != "test-secret" {
					t.Fatal("incorrect MinIO configuration")
				}
			} else if err == nil || strings.Contains(err.Error(), "invalid-private-value") {
				t.Fatal("expected safe configuration error")
			}
		})
	}
	for _, name := range []string{"MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, " ")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatal("expected named missing-setting error")
			}
		})
	}
}
