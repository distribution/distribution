// Package middleware provides a storage middleware which appends a Cloudflare
// timed-HMAC token to redirect URLs, so that a WAF rule calling
// is_timed_hmac_valid_v0() can validate them at the edge.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	storagemiddleware "github.com/distribution/distribution/v3/registry/storage/driver/middleware"
	"github.com/sirupsen/logrus"
)

const defaultParam = "verify"

var (
	errSecretConflict = errors.New("secret and secretfile are mutually exclusive")
	errNoSecret       = errors.New("secret or secretfile must be provided")
	errInvalidParam   = errors.New("param must be a valid query parameter name")
)

func init() {
	if err := storagemiddleware.Register("cdnhmac", newCDNHMACStorageMiddleware); err != nil {
		logrus.Errorf("failed to register cdnhmac storage middleware: %v", err)
	}
}

type cdnHMACStorageMiddleware struct {
	storagedriver.StorageDriver
	param  string
	secret []byte
	// now is overridable in tests.
	now func() time.Time
}

var _ storagedriver.StorageDriver = &cdnHMACStorageMiddleware{}

func getStringOption(key string, options map[string]any) (string, error) {
	o, ok := options[key]
	if !ok {
		return "", nil
	}

	s, ok := o.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}

	return s, nil
}

func newCDNHMACStorageMiddleware(_ context.Context, sd storagedriver.StorageDriver, options map[string]any) (storagedriver.StorageDriver, error) {
	m := &cdnHMACStorageMiddleware{StorageDriver: sd, now: time.Now}

	secret, err := getStringOption("secret", options)
	if err != nil {
		return nil, err
	}

	secretFile, err := getStringOption("secretfile", options)
	if err != nil {
		return nil, err
	}

	switch {
	case secret != "" && secretFile != "":
		return nil, errSecretConflict
	case secretFile != "":
		raw, err := os.ReadFile(secretFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read secretfile: %w", err)
		}

		m.secret = []byte(strings.TrimSpace(string(raw)))
		if len(m.secret) == 0 {
			return nil, fmt.Errorf("secretfile %q is empty", secretFile)
		}
	case secret != "":
		m.secret = []byte(secret)
	default:
		return nil, errNoSecret
	}

	if m.param, err = getStringOption("param", options); err != nil {
		return nil, err
	}

	if m.param == "" {
		m.param = defaultParam
	}

	// the token is appended unescaped, so the parameter name must not need escaping either.
	if url.QueryEscape(m.param) != m.param {
		return nil, errInvalidParam
	}

	return m, nil
}

// token builds a timed-HMAC token over message at issue time ts.
//
// Format: "<unix-seconds>-<base64url(HMAC-SHA256(secret, message + unix-seconds))>", unpadded.
// The WAF rule validating it must pass flags 's' to select that alphabet.
func token(secret []byte, message string, ts time.Time) string {
	stamp := strconv.FormatInt(ts.Unix(), 10)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	mac.Write([]byte(stamp))

	return stamp + "-" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *cdnHMACStorageMiddleware) RedirectURL(req *http.Request, path string) (string, error) {
	redirectURL, err := m.StorageDriver.RedirectURL(req, path)
	if err != nil {
		return "", err
	}

	// the driver declined to presign, so the registry will stream the blob itself.
	if redirectURL == "" {
		return "", nil
	}

	u, err := url.Parse(redirectURL)
	if err != nil {
		return "", err
	}

	// Cloudflare's is_timed_hmac_valid_v0() parses http.request.uri positionally from the
	// end: MAC, timestamp, then lengthOfSeparator bytes. Everything before that is the
	// message. So the token must be last in the query string, and the message is the
	// whole URI preceding it, including any presigned query parameters.
	//
	// This middleware must be listed after any middleware that rewrites the path, because
	// the message has to match the URI the client actually requests.
	message := u.EscapedPath()
	if u.RawQuery != "" {
		message += "?" + u.RawQuery
	}

	// RawURLEncoding output needs no escaping, so the existing query is only ever appended
	// to. Round-tripping it through url.Values would reorder it and break the SigV4 signature.
	t := m.param + "=" + token(m.secret, message, m.now())

	if u.RawQuery == "" {
		u.RawQuery = t
	} else {
		u.RawQuery += "&" + t
	}

	return u.String(), nil
}
