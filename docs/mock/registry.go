package mock

import (
	"github.com/docker/orca"
	"net/http"
	"net/url"
)

type (
	MockRegistry struct {
		orca.RegistryConfig
		client *orca.RegistryClient
	}
)

func NewRegistry(reg *orca.RegistryConfig) (orca.Registry, error) {
	u, err := url.Parse(reg.URL)
	if err != nil {
		return nil, err
	}

	rClient := &orca.RegistryClient{
		URL: u,
	}

	return &MockRegistry{
		RegistryConfig: *reg,
		client:         rClient,
	}, nil
}

func (r *MockRegistry) GetAuthToken(username, accessType, hostname, reponame string) (string, error) {
	return "foo", nil
}

func (r *MockRegistry) GetConfig() *orca.RegistryConfig {
	return &r.RegistryConfig
}

func (r *MockRegistry) GetTransport() http.RoundTripper {
	return r.client.HttpClient.Transport
}
