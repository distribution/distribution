package handlers

import "testing"

func TestMailMessage(t *testing.T) {
	m := &mailer{From: "registry@example.com", To: []string{"a@example.com", "b@example.com"}}
	got := string(m.message("subject", "body"))
	want := "To: a@example.com, b@example.com\r\n" +
		"From: registry@example.com\r\n" +
		"Subject: subject\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"body"
	if got != want {
		t.Fatalf("got message %q, want %q", got, want)
	}
}
