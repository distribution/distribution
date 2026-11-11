package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/sirupsen/logrus"
)

func BenchmarkBlobUploadPatch(b *testing.B) {
	output := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	b.Cleanup(func() { logrus.SetOutput(output) })

	config := &configuration.Configuration{
		Storage: configuration.Storage{
			"filesystem": configuration.Parameters{"rootdirectory": b.TempDir()},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.HTTP.Secret = "benchmark-upload-secret"
	app := NewApp(context.Background(), config)
	b.Cleanup(app.cancel)
	payload := []byte{1, 2, 3}

	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for range b.N {
		post := httptest.NewRecorder()
		app.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "http://registry/v2/bench/upload/blobs/uploads/", nil))
		if post.Code != http.StatusAccepted {
			b.Fatalf("POST: %d: %s", post.Code, post.Body.String())
		}

		req := httptest.NewRequest(http.MethodPatch, post.Header().Get("Location"), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/octet-stream")
		patch := httptest.NewRecorder()
		b.StartTimer()
		app.ServeHTTP(patch, req)
		b.StopTimer()
		if patch.Code != http.StatusAccepted {
			b.Fatalf("PATCH: %d: %s", patch.Code, patch.Body.String())
		}

		cancel := httptest.NewRecorder()
		app.ServeHTTP(cancel, httptest.NewRequest(http.MethodDelete, patch.Header().Get("Location"), nil))
		if cancel.Code != http.StatusNoContent {
			b.Fatalf("DELETE: %d: %s", cancel.Code, cancel.Body.String())
		}
	}
}
