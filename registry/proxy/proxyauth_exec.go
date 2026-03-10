package proxy

import (
	"net/url"
	"sync"
	"time"

	"github.com/docker/docker-credential-helpers/client"
	"github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client/auth"
)

type execCredentials struct {
	m        sync.Mutex
	helper   client.ProgramFunc
	lifetime *time.Duration
	creds    map[string]execCredentialsCredential
}

type execCredentialsCredential struct {
	username string
	secret   string
	expiry   time.Time
}

func (c *execCredentials) Basic(url *url.URL) (string, string) {
	c.m.Lock()
	defer c.m.Unlock()

	now := time.Now()
	creds, ok := c.creds[url.Host]
	if ok && (c.lifetime == nil || now.Before(creds.expiry)) {
		return creds.username, creds.secret
	}

	helperCreds, err := client.Get(c.helper, url.Host)
	if err != nil {
		logrus.Errorf("Failed to run credential helper command: %v", err)
		return "", ""
	}

	creds = execCredentialsCredential{
		username: helperCreds.Username,
		secret:   helperCreds.Secret,
	}
	if c.lifetime != nil && *c.lifetime > 0 {
		creds.expiry = now.Add(*c.lifetime)
	}
	c.creds[url.Host] = creds

	return creds.username, creds.secret
}

func (c *execCredentials) RefreshToken(_ *url.URL, _ string) string {
	return ""
}

func (c *execCredentials) SetRefreshToken(_ *url.URL, _, _ string) {
}

func configureExecAuth(cfg configuration.ExecConfig) (auth.CredentialStore, error) {
	return &execCredentials{
		helper:   client.NewShellProgramFunc(cfg.Command),
		lifetime: cfg.Lifetime,
		creds:    make(map[string]execCredentialsCredential),
	}, nil
}
