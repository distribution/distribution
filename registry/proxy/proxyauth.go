package proxy

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client/auth"
	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/internal/dcontext"
)

const challengeHeader = "Docker-Distribution-Api-Version"

type userpass struct {
	username string
	password string
}

type credentials struct {
	creds map[string]userpass
}

func (c credentials) Basic(u *url.URL) (string, string) {
	up := c.creds[u.Host]

	return up.username, up.password
}

func (c credentials) RefreshToken(_ *url.URL, _ string) string {
	return ""
}

func (c credentials) SetRefreshToken(_ *url.URL, _, _ string) {
}

// configureAuth stores credentials for challenge responses
func configureAuth(configCredentials map[string]configuration.ProxyCredential) (auth.CredentialStore, error) {
	creds := map[string]userpass{}

	for remoteURLString, credential := range configCredentials {
		remoteURL, err := parseSchemelessURL(remoteURLString)
		if err != nil {
			return nil, err
		}

		creds[remoteURL.Host] = userpass{
			username: credential.Username,
			password: credential.Password,
		}
		dcontext.GetLogger(dcontext.Background()).Infof("Registered credentials for remote registry: %s", remoteURL.Host)
	}

	return credentials{creds: creds}, nil
}

func parseSchemelessURL(u string) (*url.URL, error) {
	if !(strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		u = "https://" + u
	}
	return url.Parse(u)
}

func ping(manager challenge.Manager, endpoint, versionHeader string) error {
	resp, err := http.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return manager.AddResponse(resp)
}
