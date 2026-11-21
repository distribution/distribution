package gcs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestEmulatorUploadAPIBase(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "")
		u, err := emulatorUploadAPIBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u != nil {
			t.Fatalf("expected nil base, got %v", u)
		}
	})

	t.Run("host without scheme", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "localhost:9000")
		u, err := emulatorUploadAPIBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u.Scheme != "http" || u.Host != "localhost:9000" {
			t.Fatalf("unexpected base: %v", u)
		}
	})

	t.Run("host with scheme", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "http://127.0.0.1:9000/")
		u, err := emulatorUploadAPIBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u.Scheme != "http" || u.Host != "127.0.0.1:9000" {
			t.Fatalf("unexpected base: %v", u)
		}
		if u.Path != "" {
			t.Fatalf("expected path stripped, got %q", u.Path)
		}
	})

	t.Run("invalid missing host", func(t *testing.T) {
		t.Setenv("STORAGE_EMULATOR_HOST", "http://")
		_, err := emulatorUploadAPIBase()
		if err == nil {
			t.Fatal("expected error for missing host")
		}
	})
}

func TestResumableUploadURL(t *testing.T) {
	t.Run("production", func(t *testing.T) {
		u := resumableUploadURL(nil, "my-bucket", "path/to/object")
		if got, want := u.String(), "https://www.googleapis.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable&name=path/to/object"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("emulator", func(t *testing.T) {
		base := &url.URL{Scheme: "http", Host: "localhost:9000"}
		u := resumableUploadURL(base, "my-bucket", "path/to/object")
		if got, want := u.String(), "http://localhost:9000/upload/storage/v1/b/my-bucket/o?uploadType=resumable&name=path/to/object"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestFromParametersEmulatorResumableUpload(t *testing.T) {
	t.Run("no credentials", func(t *testing.T) {
		testFromParametersEmulatorResumableUpload(t, nil)
	})

	t.Run("keyfile credentials", func(t *testing.T) {
		keyPath := writeTestServiceAccountKey(t)
		testFromParametersEmulatorResumableUpload(t, map[string]any{
			"keyfile": keyPath,
		})
	})

	t.Run("inline credentials", func(t *testing.T) {
		creds := testServiceAccountCredentialsMap(t)
		testFromParametersEmulatorResumableUpload(t, map[string]any{
			"credentials": creds,
		})
	})
}

func testFromParametersEmulatorResumableUpload(t *testing.T, extraParams map[string]any) {
	t.Helper()

	var (
		mu           sync.Mutex
		sessionPosts int
		chunkPuts    int
		authHeaders  []string
		sessionURL   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if auth := r.Header.Get("Authorization"); auth != "" {
			authHeaders = append(authHeaders, auth)
		}
		mu.Unlock()

		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/") && r.URL.Query().Get("uploadType") == "resumable":
			mu.Lock()
			sessionPosts++
			sessionURL = r.URL.String()
			mu.Unlock()
			w.Header().Set("Location", "http://"+r.Host+"/upload/session/1")
			w.WriteHeader(http.StatusOK)
			return

		case r.Method == http.MethodPut && r.URL.Path == "/upload/session/1":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			mu.Lock()
			chunkPuts++
			mu.Unlock()

			// Incomplete chunk uploads use an unknown total size ("*").
			if strings.HasSuffix(r.Header.Get("Content-Range"), "/*") {
				if len(body) == 0 {
					http.Error(w, "expected chunk body", http.StatusBadRequest)
					return
				}
				w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(body)-1))
				w.WriteHeader(http.StatusPermanentRedirect)
				return
			}

			// Finalize (Commit) accepts empty body with bytes */N.
			w.WriteHeader(http.StatusOK)
			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv("STORAGE_EMULATOR_HOST", host)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

	params := map[string]any{
		"bucket":    "test-bucket",
		"chunksize": minChunkSize,
	}
	for k, v := range extraParams {
		params[k] = v
	}

	d, err := FromParameters(context.Background(), params)
	if err != nil {
		t.Fatalf("FromParameters: %v", err)
	}

	w, err := d.Writer(context.Background(), "/emu-upload", false)
	if err != nil {
		t.Fatalf("Writer: %v", err)
	}

	payload := make([]byte, minChunkSize)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	if n, err := w.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if err := w.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if sessionPosts != 1 {
		t.Fatalf("session POSTs: got %d, want 1 (url=%q)", sessionPosts, sessionURL)
	}
	if chunkPuts < 1 {
		t.Fatalf("chunk PUTs: got %d, want at least 1", chunkPuts)
	}
	if len(authHeaders) != 0 {
		t.Fatalf("expected no Authorization headers, got %v", authHeaders)
	}
	if !strings.Contains(sessionURL, "/upload/storage/v1/b/test-bucket/o") {
		t.Fatalf("unexpected session URL path: %q", sessionURL)
	}
	if !strings.Contains(sessionURL, "uploadType=resumable") {
		t.Fatalf("missing uploadType=resumable in %q", sessionURL)
	}
}

func writeTestServiceAccountKey(t *testing.T) string {
	t.Helper()
	data := mustTestServiceAccountJSON(t)
	f, err := os.CreateTemp(t.TempDir(), "sa-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}

func testServiceAccountCredentialsMap(t *testing.T) map[any]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(mustTestServiceAccountJSON(t), &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	out := make(map[any]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}

func mustTestServiceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	data, err := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "test-key",
		"private_key":                 string(pemKey),
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "1234567890",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}
