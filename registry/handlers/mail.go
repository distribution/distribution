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
		mail.buildMessage(subject, message),
	)
	if err != nil {
		return err
	}
	return nil
}

// buildMessage assembles the mail. subject is placed in the header block, so a
// CR or LF in it is folded to a space to keep it on a single line and stop a
// newline from injecting extra SMTP headers. subject is built from the log
// entry message and is not guaranteed to be header-safe.
func (mail *mailer) buildMessage(subject, message string) []byte {
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	return []byte("To:" + strings.Join(mail.To, ";") +
		"\r\nFrom: " + mail.From +
		"\r\nSubject: " + subject +
		"\r\nContent-Type: text/plain\r\n\r\n" +
		message)
}
