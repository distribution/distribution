package bugsnag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/bugsnag/bugsnag-go/v2/device"
	"github.com/bugsnag/bugsnag-go/v2/headers"
	"github.com/bugsnag/bugsnag-go/v2/sessions"
)

const notifyPayloadVersion = "4"

var sessionMutex sync.Mutex

type payload struct {
	*Event
	*Configuration
}

type hash map[string]interface{}

func (p *payload) deliver() error {

	if len(p.APIKey) != 32 {
		return fmt.Errorf("bugsnag/payload.deliver: invalid api key: '%s'", p.APIKey)
	}

	buf, err := p.MarshalJSON()

	if err != nil {
		return fmt.Errorf("bugsnag/payload.deliver: %v", err)
	}

	client := http.Client{
		Transport: p.Transport,
	}
	req, err := http.NewRequest("POST", p.Endpoints.Notify, bytes.NewBuffer(buf))
	if err != nil {
		return fmt.Errorf("bugsnag/payload.deliver unable to create request: %v", err)
	}
	for k, v := range headers.PrefixedHeaders(p.APIKey, notifyPayloadVersion) {
		req.Header.Add(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("bugsnag/payload.deliver: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bugsnag/payload.deliver: Got HTTP %s", resp.Status)
	}

	return nil
}

func (p *payload) MarshalJSON() ([]byte, error) {
	return json.Marshal(reportJSON{
		APIKey: p.APIKey,
		Events: []eventJSON{
			eventJSON{
				App: &appJSON{
					ReleaseStage: p.ReleaseStage,
					Type:         p.AppType,
					Version:      p.AppVersion,
				},
				Context: p.Context,
				Device: &deviceJSON{
					Hostname:        p.Hostname,
					OsName:          runtime.GOOS,
					RuntimeVersions: device.GetRuntimeVersions(),
				},
				Request: p.Request,
				Exceptions:     p.exceptions(),
				GroupingHash:   p.GroupingHash,
				Metadata:       p.MetaData.sanitize(p.ParamsFilters),
				PayloadVersion: notifyPayloadVersion,
				Session:        p.makeSession(),
				Severity:       p.Severity.String,
				SeverityReason: p.severityReasonPayload(),
				Unhandled:      p.Unhandled,
				User:           p.User,
			},
		},
		Notifier: notifierJSON{
			Name:    "Bugsnag Go",
			URL:     "https://github.com/bugsnag/bugsnag-go",
			Version: Version,
		},
	})
}

func (p *payload) makeSession() *sessionJSON {
	// If a context has not been applied to the payload then assume that no
	// session has started either
	if p.Ctx == nil {
		return nil
	}

	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	session := sessions.IncrementEventCountAndGetSession(p.Ctx, p.Unhandled)
	if session != nil {
		s := *session
		return &sessionJSON{
			ID:        s.ID,
			StartedAt: s.StartedAt.UTC().Format(time.RFC3339),
			Events: sessions.EventCounts{
				Handled:   s.EventCounts.Handled,
				Unhandled: s.EventCounts.Unhandled,
			},
		}
	}
	return nil
}

func (p *payload) severityReasonPayload() *severityReasonJSON {
	if reason := p.handledState.SeverityReason; reason != "" {
		json := &severityReasonJSON{
			Type: reason,
			UnhandledOverridden: p.handledState.Unhandled != p.Unhandled,
		}
		if p.handledState.Framework != "" {
			json.Attributes = make(map[string]string, 1)
			json.Attributes["framework"] = p.handledState.Framework
		}
		return json
	}
	return nil
}

func (p *payload) exceptions() []exceptionJSON {
	exceptions := []exceptionJSON{
		exceptionJSON{
			ErrorClass: p.ErrorClass,
			Message:    p.Message,
			Stacktrace: p.Stacktrace,
		},
	}

	if p.Error == nil {
		return exceptions
	}

	cause := p.Error.Cause
	for cause != nil {
		exceptions = append(exceptions, exceptionJSON{
			ErrorClass: cause.TypeName(),
			Message:    cause.Error(),
			Stacktrace: generateStacktrace(cause, p.Configuration),
		})
		cause = cause.Cause
	}

	return exceptions
}
