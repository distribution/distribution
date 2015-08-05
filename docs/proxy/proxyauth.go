package proxy

import (
	"net/http"
	"net/url"

	"github.com/docker/distribution/registry/client/auth"
)

const tokenURL = "https://auth.docker.io/token"

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

// ConfigureAuth authorizes with the upstream registry
func ConfigureAuth(remoteURL, username, password string, cm auth.ChallengeManager) (auth.CredentialStore, error) {
	if err := ping(cm, remoteURL+"/v2/", "Docker-Distribution-Api-Version"); err != nil {
		return nil, err
	}

	creds := map[string]userpass{
		tokenURL: {
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
