package proxy

import (
	"net/url"
	"sync"
	"time"

	"github.com/docker/docker-credential-helpers/client"
	credspkg "github.com/docker/docker-credential-helpers/credentials"
	"github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client/auth"
)

type execCredentials struct {
	m        sync.Mutex
	helper   client.ProgramFunc
	lifetime *time.Duration
	creds    *credspkg.Credentials
	expiry   time.Time
}

func (c *execCredentials) Basic(url *url.URL) (string, string) {
	c.m.Lock()
	defer c.m.Unlock()

	now := time.Now()
	if c.creds != nil && (c.lifetime == nil || now.Before(c.expiry)) {
		return c.creds.Username, c.creds.Secret
	}

	creds, err := client.Get(c.helper, url.Host)
	if err != nil {
		logrus.Errorf("failed to run command: %v", err)
		return "", ""
	}
	c.creds = creds
	if c.lifetime != nil && *c.lifetime > 0 {
		c.expiry = now.Add(*c.lifetime)
	}

	return c.creds.Username, c.creds.Secret
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
	}, nil
}
