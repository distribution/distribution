package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/distribution/distribution/v3/internal/dcontext"
)

// multiHandler fans every record out to all wrapped handlers, taking the
// place of the hook mechanism logrus provided.
type multiHandler struct {
	handlers []slog.Handler
}

var _ slog.Handler = (*multiHandler)(nil)

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, record.Level) {
			errs = append(errs, h.Handle(ctx, record.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// mailHandler is a slog.Handler that sends records logged at the configured
// levels by email, replacing the logrus mail hook.
type mailHandler struct {
	levels map[slog.Level]bool
	mail   *mailer
	attrs  []slog.Attr
	groups []string
}

var _ slog.Handler = (*mailHandler)(nil)

func newMailHandler(mail *mailer, levels []string) *mailHandler {
	enabled := make(map[slog.Level]bool, len(levels))
	for _, v := range levels {
		lv, err := dcontext.ParseLevel(v)
		if err != nil {
			slog.Warn("ignoring invalid mail hook level", "level", v, "error", err)
			continue
		}
		enabled[lv] = true
	}
	return &mailHandler{levels: enabled, mail: mail}
}

func (hook *mailHandler) Enabled(_ context.Context, level slog.Level) bool {
	return hook.levels[level]
}

// Handle forwards the record by email.
func (hook *mailHandler) Handle(_ context.Context, record slog.Record) error {
	host, _, ok := strings.Cut(hook.mail.Addr, ":")
	if !ok || host == "" {
		return errors.New("invalid Mail Address")
	}
	subject := fmt.Sprintf("[%s] %s: %s", dcontext.LevelName(record.Level), host, record.Message)

	fields := make(map[string]any, len(hook.attrs)+record.NumAttrs())
	prefix := ""
	if len(hook.groups) > 0 {
		prefix = strings.Join(hook.groups, ".") + "."
	}
	for _, attr := range hook.attrs {
		fields[prefix+attr.Key] = attr.Value.Any()
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields[prefix+attr.Key] = attr.Value.Any()
		return true
	})

	html := `
	{{.Message}}

	{{range $key, $value := .Data}}
	{{$key}}: {{$value}}
	{{end}}
	`
	b := bytes.NewBuffer(make([]byte, 0))
	t := template.Must(template.New("mail body").Parse(html))
	if err := t.Execute(b, struct {
		Message string
		Data    map[string]any
	}{Message: record.Message, Data: fields}); err != nil {
		return err
	}

	return hook.mail.sendMail(subject, b.String())
}

func (hook *mailHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h := *hook
	h.attrs = append(append([]slog.Attr{}, hook.attrs...), attrs...)
	return &h
}

func (hook *mailHandler) WithGroup(name string) slog.Handler {
	h := *hook
	h.groups = append(append([]string{}, hook.groups...), name)
	return &h
}
