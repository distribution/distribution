package registry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/docker/distribution/storage"
)

// tokenProvider contains methods for serializing and deserializing state from token strings.
type tokenProvider interface {
	// layerUploadStateFromToken retrieves the LayerUploadState for a given state token.
	layerUploadStateFromToken(stateToken string) (storage.LayerUploadState, error)

	// layerUploadStateToToken returns a token string representing the given LayerUploadState.
	layerUploadStateToToken(layerUploadState storage.LayerUploadState) (string, error)
}

type hmacTokenProvider struct {
	secret string
}

func newHMACTokenProvider(secret string) tokenProvider {
	return &hmacTokenProvider{secret: secret}
}

// layerUploadStateFromToken deserializes the given HMAC stateToken and validates the prefix HMAC
func (ts *hmacTokenProvider) layerUploadStateFromToken(stateToken string) (storage.LayerUploadState, error) {
	var lus storage.LayerUploadState

	tokenBytes, err := base64.URLEncoding.DecodeString(stateToken)
	if err != nil {
		return lus, err
	}
	mac := hmac.New(sha256.New, []byte(ts.secret))

	if len(tokenBytes) < mac.Size() {
		return lus, fmt.Errorf("Invalid token")
	}

	macBytes := tokenBytes[:mac.Size()]
	messageBytes := tokenBytes[mac.Size():]

	mac.Write(messageBytes)
	if !hmac.Equal(mac.Sum(nil), macBytes) {
		return lus, fmt.Errorf("Invalid token")
	}

	if err := json.Unmarshal(messageBytes, &lus); err != nil {
		return lus, err
	}

	return lus, nil
}

// layerUploadStateToToken serializes the given LayerUploadState to JSON with an HMAC prepended
func (ts *hmacTokenProvider) layerUploadStateToToken(lus storage.LayerUploadState) (string, error) {
	mac := hmac.New(sha256.New, []byte(ts.secret))
	stateJSON := fmt.Sprintf("{\"Name\": \"%s\", \"UUID\": \"%s\", \"Offset\": %d}", lus.Name, lus.UUID, lus.Offset)
	mac.Write([]byte(stateJSON))
	return base64.URLEncoding.EncodeToString(append(mac.Sum(nil), stateJSON...)), nil
}
