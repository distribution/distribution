package handlers

import (
	"errors"
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
	addr := strings.Split(mail.Addr, ":")
	if len(addr) != 2 {
		return errors.New("invalid Mail Address")
	}
	host := addr[0]
	msg := mail.message(subject, message)
	auth := smtp.PlainAuth(
		"",
		mail.Username,
		mail.Password,
		host,
	)
	err := smtp.SendMail(
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

// message builds the mail sent by sendMail. RFC 5322 separates the addresses
// of the To header with commas.
func (mail *mailer) message(subject, body string) []byte {
	return []byte("To: " + strings.Join(mail.To, ", ") +
		"\r\nFrom: " + mail.From +
		"\r\nSubject: " + subject +
		"\r\nContent-Type: text/plain\r\n\r\n" +
		body)
}
