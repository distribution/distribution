package handlers

import (
	"errors"
	"net"
	"net/smtp"
	"strings"
)

// mailer provides fields of email configuration for sending.
type mailer struct {
	Addr, Username, Password, From string
	Insecure                       bool
	To                             []string
}

// sendMail allows users to send email, only if mail parameters is configured correctly.
func (mail *mailer) sendMail(subject, message string) error {
	// SplitHostPort also accepts IPv6 addresses such as "[2001:db8::1]:587".
	host, _, err := net.SplitHostPort(mail.Addr)
	if err != nil {
		return errors.New("invalid Mail Address")
	}
	msg := []byte("To:" + strings.Join(mail.To, ";") +
		"\r\nFrom: " + mail.From +
		"\r\nSubject: " + subject +
		"\r\nContent-Type: text/plain\r\n\r\n" +
		message)
	auth := smtp.PlainAuth(
		"",
		mail.Username,
		mail.Password,
		host,
	)
	err = smtp.SendMail(
		mail.Addr,
		auth,
		mail.From,
		mail.To,
		msg,
	)
	if err != nil {
		return err
	}
	return nil
}
