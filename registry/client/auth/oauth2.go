package auth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// IsNotSupported checks whether the error indicates that
// the operation is not supported for the given oauth2 parameters.
func IsNotSupported(err error) bool {
	switch err.(type) {
	case invalidOAuth2Scheme:
		return true
	case callbackListenErr:
		return true
	}
	return false
}

type invalidOAuth2Scheme struct {
	scheme string
}

func (err invalidOAuth2Scheme) Error() string {
	return fmt.Sprintf("invalid OAuth2 scheme: %s", err.scheme)
}

type callbackListenErr struct {
	listenErr error
}

func (err callbackListenErr) Error() string {
	return fmt.Sprintf("unable to listen for callback: %s", err.listenErr)
}

// OAuth2Config represents a generic set of oauth2 parameters a client
// will need to direct a user through the authorization flow to get
// an authorization code.
type OAuth2Config struct {
	ClientID    string   `json:"client_id"`
	AuthURL     string   `json:"auth_url"`
	CallbackURL string   `json:"callback_url,omitempty"`
	CodeURL     string   `json:"code_url,omitempty"`
	LandingURL  string   `json:"landing_url,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

// HeaderValue returns the value for an authentication header.
// This information will be parsed and used by clients.
func (c OAuth2Config) HeaderValue() (str string) {
	str = fmt.Sprintf("OAuth2 client_id=%q,auth_url=%q", c.ClientID, c.AuthURL)
	if c.CallbackURL != "" {
		str = fmt.Sprintf("%s,callback_url=%q", str, c.CallbackURL)
	}
	if c.CodeURL != "" {
		str = fmt.Sprintf("%s,code_url=%q", str, c.CodeURL)
	}
	if c.LandingURL != "" {
		str = fmt.Sprintf("%s,landing_url=%q", str, c.LandingURL)
	}
	if len(c.Scopes) > 0 {
		str = fmt.Sprintf("%s,scopes=%q", str, strings.Join(c.Scopes, " "))
	}

	return
}

// GetOAuth2Config gets an OAuth2 configuration using a bearer challenge.
// The realm is queried for an oauth2 configuration.
func GetOAuth2Config(client *http.Client, ch Challenge) (OAuth2Config, error) {
	if ch.Scheme != "bearer" {
		return OAuth2Config{}, invalidOAuth2Scheme{ch.Scheme}
	}

	realm, ok := ch.Parameters["realm"]
	if !ok {
		return OAuth2Config{}, errors.New("no realm specified for token auth challenge")
	}

	resp, err := client.Head(realm)
	if err != nil {
		return OAuth2Config{}, err
	}
	defer resp.Body.Close()

	value, params := parseValueAndParams(resp.Header.Get("WWW-Authenticate"))
	if value != "oauth2" {
		return OAuth2Config{}, errors.New("missing oauth2 config header")
	}

	config := OAuth2Config{
		ClientID:    params["client_id"],
		AuthURL:     params["auth_url"],
		CallbackURL: params["callback_url"],
		LandingURL:  params["landing_url"],
		CodeURL:     params["code_url"],
	}
	if scopes := params["scopes"]; scopes != "" {
		config.Scopes = strings.Split(scopes, " ")
	}

	return config, nil
}

// OAuth2CodeHandler handles getting an oauth code back to
// a client from an oauth2 user agent.
type OAuth2CodeHandler interface {
	CodeChan() <-chan string
	Error() error
	Cancel(error)
}

type callbackHandler struct {
	authURL *url.URL

	shutdown chan struct{}
	errL     sync.Mutex
	err      error

	codeChan chan string
	state    string
	redirect string
}

// NewOAuth2CallbackHandler sets up a handler to listen at a callback url
// and return the authorization code return from the authorization process.
func NewOAuth2CallbackHandler(callbackURL, state, redirect string) (OAuth2CodeHandler, error) {
	authURL, err := url.Parse(callbackURL)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", authURL.Host)
	if err != nil {
		return nil, callbackListenErr{err}
	}

	if redirect == "" {
		redirect = "https://docs.docker.com/registry/"
	}

	codeChan := make(chan string, 1)
	shutdownChan := make(chan struct{})

	handler := &callbackHandler{
		authURL:  authURL,
		codeChan: codeChan,
		shutdown: shutdownChan,
		state:    state,
		redirect: redirect,
	}

	go http.Serve(listener, handler)
	go func() {
		<-shutdownChan
		time.Sleep(time.Second)
		listener.Close()
	}()

	return handler, nil
}

func (h *callbackHandler) CodeChan() <-chan string {
	return h.codeChan
}

func (h *callbackHandler) Error() error {
	h.errL.Lock()
	defer h.errL.Unlock()
	return h.err
}

func (h *callbackHandler) Cancel(err error) {
	h.errL.Lock()
	defer h.errL.Unlock()
	if h.shutdown != nil {
		close(h.shutdown)
		close(h.codeChan)
		h.shutdown = nil
		h.err = err
	}
}

// ServeHTTP serves the callback handler for an http client
// to redirect a user to after authorization.
func (h *callbackHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != h.authURL.Path {
		http.Error(rw, "Unexpected Path", http.StatusNotFound)
		return
	}

	state := r.URL.Query().Get("state")
	if state != h.state {
		http.Error(rw, "Bad state", http.StatusBadRequest)
		return
	}

	http.Redirect(rw, r, h.redirect, http.StatusMovedPermanently)

	code := r.URL.Query().Get("code")
	if code == "" {
		return
	}

	h.errL.Lock()
	if h.shutdown != nil {
		h.codeChan <- code
		close(h.shutdown)
		h.shutdown = nil
	}
	h.errL.Unlock()
}

type pollHandler struct {
	codeReq *http.Request
	client  *http.Client

	errL      sync.Mutex
	err       error
	shutdownC chan struct{}
	shutdown  bool

	codeChan chan string
	state    string
}

// NewOAuth2PollHandler gets the code by polling the code URL with the
// provided state and waits for the code to be returned. Each request
// will long poll to wait for the code to become available.
func NewOAuth2PollHandler(client *http.Client, codeURL, state string) (OAuth2CodeHandler, error) {
	req, err := http.NewRequest("GET", codeURL, nil)
	if err != nil {
		return nil, err
	}
	query := req.URL.Query()
	query.Set("state", state)
	req.URL.RawQuery = query.Encode()

	codeChan := make(chan string, 1)
	shutdownChan := make(chan struct{})

	handler := &pollHandler{
		codeReq:   req,
		client:    client,
		codeChan:  codeChan,
		shutdownC: shutdownChan,
		state:     state,
	}

	go handler.poll()

	return handler, nil
}

func (h *pollHandler) CodeChan() <-chan string {
	return h.codeChan
}

func (h *pollHandler) Error() error {
	h.errL.Lock()
	defer h.errL.Unlock()
	return h.err
}

func (h *pollHandler) Cancel(err error) {
	h.errL.Lock()
	defer h.errL.Unlock()
	if !h.shutdown {
		close(h.shutdownC)
		close(h.codeChan)
		h.shutdown = true
		h.err = err
	}
}

func (h *pollHandler) poll() {
	var (
		code    string
		codeErr error
		retry   int
	)
	for retry < 3 {
		codeErr = nil
		resp, err := h.client.Do(h.codeReq)
		if err != nil {
			// TODO: retry on temporary network errors
			codeErr = err
			break
		}
		if resp.StatusCode != 200 {
			if resp.StatusCode >= 500 {
				retry++
				codeErr = fmt.Errorf("server error: %s", resp.Status)
				time.Sleep(time.Second)
				continue
			}
			codeErr = fmt.Errorf("unable to get code: %s", resp.Status)
			break
		}
		codeC := make(chan string, 1)
		var scanErr error

		go func() {
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				if scanErr = scanner.Err(); scanErr != nil {
					close(codeC)
					break
				}
				t := scanner.Text()
				if strings.HasPrefix(t, "CODE ") {
					codeC <- t[5:]
					break
				}
			}
		}()

		select {
		case <-h.shutdownC:
		case code = <-codeC:
			if scanErr != nil {
				codeErr = fmt.Errorf("error reading body: %s", scanErr)
				if scanErr == io.EOF {
					retry++
					continue
				}
			}
		}
		resp.Body.Close()
		break
	}
	h.errL.Lock()
	if !h.shutdown {
		if code != "" {
			h.codeChan <- code
		} else {
			close(h.codeChan)
		}
		if codeErr != nil {
			h.err = codeErr
		}
		close(h.shutdownC)
		h.shutdown = true
	}
	h.errL.Unlock()
}
