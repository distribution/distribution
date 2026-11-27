package handlers

import (
	"net"
	"testing"
)

func TestSendMailAddress(t *testing.T) {
	// A local port with nothing listening on it, so that a valid address fails to connect
	// instead of sending mail.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		addr    string
		invalid bool
	}{
		{addr: "127.0.0.1:" + port},
		{addr: "localhost:" + port},
		{addr: "[::1]:" + port},
		{addr: "localhost", invalid: true},
		{addr: "", invalid: true},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			m := &mailer{Addr: tc.addr, From: "registry@example.com", To: []string{"admin@example.com"}}
			err := m.sendMail("subject", "message")
			if err == nil {
				t.Fatal("expected an error, nothing is listening on the port")
			}
			if invalid := err.Error() == "invalid Mail Address"; invalid != tc.invalid {
				t.Fatalf("address %q: got error %q, want invalid address error: %v", tc.addr, err, tc.invalid)
			}
		})
	}
}
