package proxy

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/stretchr/testify/require"
)

func TestGetHTTPTransport(t *testing.T) {
	tmpDir := t.TempDir()

	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Create dummy certificate files
	err := os.WriteFile(certPath, []byte("dummy cert"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, []byte("dummy key"), 0644)
	require.NoError(t, err)

	remoteURL, err := url.Parse("https://example.com")
	require.NoError(t, err)
	httpURL, err := url.Parse("http://example.com")
	require.NoError(t, err)

	tests := []struct {
		name      string
		cfg       *configuration.ProxyTLS
		remoteURL *url.URL
		wantErr   bool
	}{
		{
			name:      "nil config",
			cfg:       nil,
			remoteURL: remoteURL,
			wantErr:   false,
		},
		{
			name:      "empty config",
			cfg:       &configuration.ProxyTLS{},
			remoteURL: remoteURL,
			wantErr:   false,
		},
		{
			name: "valid TLS config but http URL",
			cfg: &configuration.ProxyTLS{
				Certificate: certPath,
				Key:         keyPath,
			},
			remoteURL: httpURL,
			wantErr:   true,
		},
		{
			name: "missing cert file",
			cfg: &configuration.ProxyTLS{
				Certificate: "nonexistent.crt",
				Key:         keyPath,
			},
			remoteURL: remoteURL,
			wantErr:   true,
		},
		{
			name: "missing key file",
			cfg: &configuration.ProxyTLS{
				Certificate: certPath,
				Key:         "nonexistent.key",
			},
			remoteURL: remoteURL,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getHttpTransport(tt.cfg, tt.remoteURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)

			if tt.cfg == nil {
				require.Equal(t, http.DefaultTransport, got)
			}
		})
	}
}
