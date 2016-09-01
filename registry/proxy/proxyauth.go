package proxy

import (
	"net/http"
	"net/url"

	"github.com/docker/distribution/registry/client/auth"
)

const tokenURL = "https://auth.docker.io/token"
const challengeHeader = "Docker-Distribution-Api-Version"

type userpass struct {
	username string
	password string
}

type credentials struct {
	creds map[string]userpass
}

func (c credentials) Basic(u *url.URL) (string, string) {
	up := c.creds[u.String()]

	return up.username, up.password
}

func (c credentials) RefreshToken(u *url.URL, service string) string {
	return ""
}

func (c credentials) SetRefreshToken(u *url.URL, service, token string) {
}

// configureAuth stores credentials for challenge responses
func configureAuth(username, password, authURL string) (auth.CredentialStore, error) {
	if authURL == "" {
		authURL = tokenURL
	}

	creds := map[string]userpass{
		authURL: {
			username: username,
			password: password,
		},
	}
	return credentials{creds: creds}, nil
}

func ping(manager auth.ChallengeManager, endpoint, versionHeader string) error {
	resp, err := http.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := manager.AddResponse(resp); err != nil {
		return err
	}

	return nil
}
