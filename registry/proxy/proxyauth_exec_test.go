package proxy

import (
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/docker/docker-credential-helpers/client"
	credspkg "github.com/docker/docker-credential-helpers/credentials"
)

type testHelper struct {
	username string
	secret   string
	err      error
}

func (h *testHelper) Output() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Username":%q,"Secret":%q}`, h.username, h.secret)), h.err
}

func (h *testHelper) Input(in io.Reader) {
}

var _ client.Program = (*testHelper)(nil)

func TestExecAuth(t *testing.T) {
	ptrDuration := func(t time.Duration) *time.Duration { return &t }

	for _, tc := range []struct {
		name         string
		helper       client.ProgramFunc
		lifetime     *time.Duration
		currCreds    *credspkg.Credentials
		currExpiry   time.Time
		wantUsername string
		wantPassword string
		wantExpiry   time.Time
	}{{
		name: "first auth without lifetime",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		wantUsername: "user",
		wantPassword: "nextpass",
	}, {
		name: "first auth with zero lifetime",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		lifetime:     ptrDuration(0),
		wantUsername: "user",
		wantPassword: "nextpass",
	}, {
		name: "first auth with lifetime",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		lifetime:     ptrDuration(time.Hour),
		wantUsername: "user",
		wantPassword: "nextpass",
		wantExpiry:   time.Now().Add(time.Hour),
	}, {
		name: "re-auth without lifetime",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		currCreds: &credspkg.Credentials{
			Username: "user",
			Secret:   "currpass",
		},
		wantUsername: "user",
		wantPassword: "currpass",
	}, {
		name: "re-auth with zero lifetime",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		lifetime: ptrDuration(0),
		currCreds: &credspkg.Credentials{
			Username: "user",
			Secret:   "currpass",
		},
		wantUsername: "user",
		wantPassword: "nextpass",
	}, {
		name: "re-auth when not expired",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		lifetime: ptrDuration(time.Hour),
		currCreds: &credspkg.Credentials{
			Username: "user",
			Secret:   "currpass",
		},
		currExpiry:   time.Now().Add(time.Minute),
		wantUsername: "user",
		wantPassword: "currpass",
		wantExpiry:   time.Now().Add(time.Minute),
	}, {
		name: "re-auth when expired",
		helper: func(...string) client.Program {
			return &testHelper{
				username: "user",
				secret:   "nextpass",
			}
		},
		lifetime: ptrDuration(time.Hour),
		currCreds: &credspkg.Credentials{
			Username: "user",
			Secret:   "currpass",
		},
		currExpiry:   time.Now().Add(-1),
		wantUsername: "user",
		wantPassword: "nextpass",
		wantExpiry:   time.Now().Add(time.Hour),
	}, {
		name: "exec error",
		helper: func(...string) client.Program {
			return &testHelper{
				err: fmt.Errorf("exec error"),
			}
		},
		lifetime: ptrDuration(time.Hour),
		currCreds: &credspkg.Credentials{
			Username: "user",
			Secret:   "currpass",
		},
		currExpiry:   time.Now().Add(-1),
		wantUsername: "",
		wantPassword: "",
		wantExpiry:   time.Now().Add(-1),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			cs := &execCredentials{
				helper:   tc.helper,
				lifetime: tc.lifetime,
				creds:    tc.currCreds,
				expiry:   tc.currExpiry,
			}
			url := &url.URL{
				Scheme: "https",
				Host:   "example.com",
			}
			user, pass := cs.Basic(url)
			if user != tc.wantUsername || pass != tc.wantPassword {
				t.Errorf("execCredentials.Basic(%q) = (%q, %q), want (%q, %q)", url, user, pass, tc.wantUsername, tc.wantPassword)
			}
			// All tests should finish within seconds, so the time error should be less than a minute.
			if cs.expiry.Sub(tc.wantExpiry).Abs() > time.Minute {
				t.Errorf("execCredentials.expiry = %v, want %v", cs.expiry, tc.wantExpiry)
			}
		})
	}
}
