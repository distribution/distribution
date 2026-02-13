package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"text/template"
)

type mailHandler struct {
	Addr, Username, Password, From string
	Insecure                       bool
	To                             []string
	slog.Handler
	levels []slog.Level
}

func NewMailHandler(
	addr, username, password, from string,
	insecure bool,
	to []string,
	handler slog.Handler,
	levels []slog.Level,
) *mailHandler {
	return &mailHandler{
		Addr:     addr,
		Username: username,
		Password: password,
		From:     from,
		Insecure: insecure,
		To:       to,
		Handler:  handler,
		Levels:   levels,
	}
}

// Handle allows users to send email, only if mail parameters is configured correctly.
func (mail *mailer) Handle(ctx context, record slog.Record) error {
	host, _, ok := strings.Cut(mail.Addr, ":")
	if !ok || host == "" {
		return errors.New("invalid Mail Address")
	}
	subject := fmt.Sprintf("[%s] %s: %s", record.Level, host, record.Message)

	html := `
	{{.Message}}

	{{range $key, $value := .Data}}
	{{$key}}: {{$value}}
	{{end}}
	`
	b := bytes.NewBuffer(make([]byte, 0))
	t := template.Must(template.New("mail body").Parse(html))
	if err := t.Execute(b, record); err != nil {
		return err
	}
	subject := ""
	message := b.String()
	addr := strings.Split(mail.Addr, ":")
	if len(addr) != 2 {
		return errors.New("invalid Mail Address")
	}
	host := addr[0]
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
	return mail.Handler.Handle(ctx, record)
}

func (mail *mailHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewMailHandler(mail.Handler.WithAttrs(attrs), mail.levels)
}

func (mail *mailHandler) WithGroup(name string) slog.Handler {
	return NewMailHandler(mail.Handler.WithGroup(name), mail.levels)
}
