package v2

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/orca"
	"github.com/docker/orca/auth"
)

var (
	ErrNotFound        = errors.New("Not found")
	defaultHTTPTimeout = 30 * time.Second
)

type (
	AuthToken struct {
		Token string `json:"token"`
	}

	V2Registry struct {
		orca.RegistryConfig
		client *orca.RegistryClient
	}
)

func NewRegistry(reg *orca.RegistryConfig, swarmTLSConfig *tls.Config) (orca.Registry, error) {
	// sanity check the registry settings
	u, err := url.Parse(reg.URL)
	if err != nil {
		return nil, fmt.Errorf("The provided Docker Trusted Registry URL was malformed and could not be parsed")
	}

	// Create a new TLS config for the registry, based on swarm's
	// This will allow us not to mess with the Swarm RootCAs
	tlsConfig := *swarmTLSConfig
	tlsConfig.InsecureSkipVerify = reg.Insecure
	if reg.CACert != "" {
		// If the user specified a CA, create a new RootCA pool containing only that CA cert.
		log.Debugf("cert: %s", reg.CACert)
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM([]byte(reg.CACert))
		tlsConfig.RootCAs = certPool
		log.Debug("Connecting to Registry with user-provided CA")
	} else {
		// If the user did not specify a CA, fall back to the system's Root CAs
		tlsConfig.RootCAs = nil
		log.Debug("Connecting to Registry with system Root CAs")
	}

	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tlsConfig},
		Timeout:   defaultHTTPTimeout,
	}

	rClient := &orca.RegistryClient{
		URL:        u,
		HttpClient: httpClient,
	}

	return &V2Registry{
		RegistryConfig: *reg,
		client:         rClient,
	}, nil
}

func (r *V2Registry) doRequest(method string, path string, body []byte, headers map[string]string, username string) ([]byte, error) {
	b := bytes.NewBuffer(body)

	req, err := http.NewRequest(method, path, b)
	if err != nil {
		log.Errorf("couldn't create request: %s", err)
		return nil, err
	}

	// The DTR Auth server will validate the UCP client cert and will grant access to whatever
	// username is passed to it.
	// However, DTR 1.4.3 rejects empty password strings under LDAP, in order to disallow anonymous users.
	req.SetBasicAuth(username, "really?")

	if headers != nil {
		for header, value := range headers {
			req.Header.Add(header, value)
		}
	}

	resp, err := r.client.HttpClient.Do(req)
	if err != nil {
		if err == http.ErrHandlerTimeout {
			log.Error("Login timed out to Docker Trusted Registry")
			return nil, err
		}
		log.Errorf("There was an error while authenticating: %s", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		// Unauthorized
		log.Warnf("Unauthorized")
		return nil, auth.ErrUnauthorized
	} else if resp.StatusCode >= 400 {
		log.Errorf("Docker Trusted Registry returned an unexpected status code while authenticating: %s", resp.Status)
		return nil, auth.ErrUnknown
	}

	rBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("couldn't read body: %s", err)
		return nil, err
	}

	return rBody, nil
}

func (r *V2Registry) GetAuthToken(username, accessType, hostname, reponame string) (string, error) {
	uri := fmt.Sprintf("%s/auth/token?scope=repository:%s:%s&service=%s", r.RegistryConfig.URL, reponame, accessType, hostname)

	log.Debugf("contacting DTR for auth token: %s", uri)

	data, err := r.doRequest("GET", uri, nil, nil, username)
	if err != nil {
		return "", err
	}

	var token AuthToken
	if err := json.Unmarshal(data, &token); err != nil {
		return "", err
	}

	return token.Token, nil
}

func (r *V2Registry) GetConfig() *orca.RegistryConfig {
	return &r.RegistryConfig
}

func (r *V2Registry) GetTransport() http.RoundTripper {
	return r.client.HttpClient.Transport
}
