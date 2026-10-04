package handlers

import (
	"errors"
	"fmt"
	"strings"

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

	var sb strings.Builder

	sb.WriteString("\n\t")
	sb.WriteString(entry.Message)
	sb.WriteString("\n")

	for key, value := range entry.Data {
		sb.WriteString(fmt.Sprintf("\t%s: %v\n", key, value))
	}
	sb.WriteString("\t")

	body := sb.String()

	return hook.Mail.sendMail(subject, body)
}

// Levels contains hook levels to be catched
func (hook *logHook) Levels() []logrus.Level {
	levels := make([]logrus.Level, 0, len(hook.LevelsParam))
	for _, v := range hook.LevelsParam {
		lv, _ := logrus.ParseLevel(v)
		levels = append(levels, lv)
	}
	return levels
}
