package middleware

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

type cloudFrontTestDriver struct{ storagedriver.StorageDriver }

func (cloudFrontTestDriver) S3BucketKey(path string) string { return strings.TrimPrefix(path, "/") }

func TestCloudFrontRedirectURLSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := newCloudFrontStorageMiddleware(t.Context(), cloudFrontTestDriver{}, map[string]any{"baseurl": "https://download.example", "privatekey": keyPath, "keypairid": "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := d.RedirectURL(httptest.NewRequest(http.MethodGet, "https://registry.example", nil), "/blobs/test")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "https" || u.Host != "download.example" || u.Path != "/blobs/test" || q.Get("Key-Pair-Id") != "test-key" {
		t.Fatalf("unexpected signed URL: %s", signed)
	}
	expires, err := strconv.ParseInt(q.Get("Expires"), 10, 64)
	if err != nil || time.Until(time.Unix(expires, 0)) < 19*time.Minute {
		t.Fatal("bad expiry")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.NewReplacer("-", "+", "_", "=", "~", "/").Replace(q.Get("Signature")))
	if err != nil {
		t.Fatal(err)
	}
	policy := fmt.Sprintf(`{"Statement":[{"Resource":"https://download.example/blobs/test","Condition":{"DateLessThan":{"AWS:EpochTime":%d}}}]}`, expires)
	digest := sha1.Sum([]byte(policy))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA1, digest[:], signature); err != nil {
		t.Fatal(err)
	}
}
