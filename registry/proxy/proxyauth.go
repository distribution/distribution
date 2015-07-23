package proxy

import (
	"net/http"

	"fmt"
	"github.com/docker/distribution/registry/client/auth"
	"net/url"
)

var cs auth.CredentialStore
var scm auth.ChallengeManager
var remoteURL string

const tokenURL = "https://auth.docker.io/token"

func init() {
	scm = auth.NewSimpleChallengeManager()
}

type credentials struct {
	creds map[string][2]string
}

func (c credentials) Basic(u *url.URL) (string, string) {
	unamePass := c.creds[u.String()]
	return unamePass[0], unamePass[1]
}

func configureAuth(options map[string]interface{}) error {
	theurl, ok := options["remoteurl"].(string)
	if !ok {
		return fmt.Errorf("Invalid format for remote url")
	}

	_, err := url.Parse(theurl)
	if err != nil {
		return err
	}
	remoteURL = theurl

	if err := ping(scm, remoteURL+"/v2/", "Docker-Distribution-Api-Version"); err != nil {
		return err
	}

	username, ok := options["username"].(string)
	if !ok {
		username = ""
	}
	password, ok := options["password"].(string)
	if !ok {
		password = ""
	}

	creds := make(map[string][2]string)
	creds[tokenURL] = [2]string{username, password}
	cs = credentials{creds: creds}

	return nil
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
