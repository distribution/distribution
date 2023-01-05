package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
)

// logHook is for hooking Panic in web application
type logHook struct {
	LevelsParam []string
	Mail        *mailer
}

// Fire forwards an error to LogHook
func (hook *logHook) Fire(entry *logrus.Entry) error {
	host, _, ok := strings.Cut(hook.Mail.Addr, ":")
	if !ok || host == "" {
		return errors.New("invalid Mail Address")
	}
	subject := fmt.Sprintf("[%s] %s: %s", entry.Level, host, entry.Message)

	html := `
	{{.Message}}

	{{range $key, $value := .Data}}
	{{$key}}: {{$value}}
	{{end}}
	`
	b := bytes.NewBuffer(make([]byte, 0))
	t := template.Must(template.New("mail body").Parse(html))
	if err := t.Execute(b, entry); err != nil {
		return err
	}
	body := b.String()

	return hook.Mail.sendMail(subject, body)
}

// Levels contains hook levels to be catched
func (hook *logHook) Levels() []logrus.Level {
	levels := []logrus.Level{}
	for _, v := range hook.LevelsParam {
		lv, _ := logrus.ParseLevel(v)
		levels = append(levels, lv)
	}
	return levels
}
