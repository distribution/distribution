package handlers

import (
	"strings"
	"testing"
)

func TestMailerBuildMessageFoldsSubject(t *testing.T) {
	m := &mailer{
		From: "registry@example.test",
		To:   []string{"ops@example.test"},
	}

	msg := string(m.buildMessage("alert\r\nBcc: attacker@evil.test", "the body"))

	headers, body, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok {
		t.Fatalf("message has no header/body separator: %q", msg)
	}
	for _, line := range strings.Split(headers, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("newline in subject injected a header line:\n%s", headers)
		}
	}
	if !strings.Contains(headers, "Subject: alert  Bcc: attacker@evil.test") {
		t.Errorf("subject was not folded to a single line:\n%s", headers)
	}
	if body != "the body" {
		t.Errorf("unexpected body: %q", body)
	}
}
