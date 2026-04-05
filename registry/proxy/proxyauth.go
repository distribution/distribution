package proxy

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/distribution/distribution/v3/internal/client/auth"
	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"golang.org/x/net/publicsuffix"
)

const challengeHeader = "Docker-Distribution-Api-Version"

type userpass struct {
	username string
	password string
}

func (u userpass) Basic(_ *url.URL) (string, string) {
	return u.username, u.password
}

func (u userpass) RefreshToken(_ *url.URL, service string) string {
	return ""
}

func (u userpass) SetRefreshToken(_ *url.URL, service, token string) {
}

type credentials struct {
	creds map[string]userpass
}

func (c credentials) Basic(u *url.URL) (string, string) {
	return c.creds[u.String()].Basic(u)
}

func (c credentials) RefreshToken(u *url.URL, service string) string {
	return ""
}

func (c credentials) SetRefreshToken(u *url.URL, service, token string) {
}

// configureAuth stores credentials for challenge responses
func configureAuth(username, password, remoteURL string) (auth.CredentialStore, auth.CredentialStore, error) {
	creds := map[string]userpass{}

	authURLs, err := getAuthURLs(remoteURL)
	if err != nil {
		return nil, nil, err
	}

	for _, url := range authURLs {
		dcontext.GetLogger(dcontext.Background()).Infof("Discovered token authentication URL: %s", url)
		creds[url] = userpass{
			username: username,
			password: password,
		}
	}

	return credentials{creds: creds}, userpass{username: username, password: password}, nil
}

func getAuthURLs(remoteURL string) ([]string, error) {
	authURLs := []string{}

	remote, err := url.Parse(remoteURL)
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(remoteURL + "/v2/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	for _, c := range challenge.ResponseChallenges(resp) {
		if strings.EqualFold(c.Scheme, "bearer") && realmAllowed(remote, c.Parameters["realm"]) {
			authURLs = append(authURLs, c.Parameters["realm"])
		}
	}

	return authURLs, nil
}

func realmAllowed(remote *url.URL, realm string) bool {
	realmURL, err := url.Parse(realm)
	if err != nil {
		return false
	}
	if realmURL.Host == "" || remote == nil || remote.Host == "" {
		return false
	}

	if strings.EqualFold(remote.Host, realmURL.Host) {
		return true
	}

	remoteHost := strings.ToLower(remote.Hostname())
	realmHost := strings.ToLower(realmURL.Hostname())
	if remoteHost == "" || realmHost == "" {
		return false
	}

	if isLiteralOrLocal(remoteHost) || isLiteralOrLocal(realmHost) {
		return false
	}

	return strings.EqualFold(registrableDomain(remoteHost), registrableDomain(realmHost))
}

func isLiteralOrLocal(host string) bool {
	if host == "localhost" {
		return true
	}

	return net.ParseIP(host) != nil
}

func registrableDomain(host string) string {
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}

	return domain
}

func ping(manager challenge.Manager, endpoint, versionHeader string) error {
	resp, err := http.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return manager.AddResponse(resp)
}
