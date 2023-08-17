// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otlphttpexporter // import "go.opentelemetry.io/collector/exporter/otlphttpexporter"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"time"

	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

type exporter struct {
	// Input configuration.
	config     *Config
	client     *http.Client
	tracesURL  string
	metricsURL string
	logsURL    string
	logger     *zap.Logger
	settings   component.TelemetrySettings
	// Default user-agent header.
	userAgent string
}

const (
	headerRetryAfter         = "Retry-After"
	maxHTTPResponseReadBytes = 64 * 1024
)

// Create new exporter.
func newExporter(cfg config.Exporter, set component.ExporterCreateSettings) (*exporter, error) {
	oCfg := cfg.(*Config)

	if oCfg.Endpoint != "" {
		_, err := url.Parse(oCfg.Endpoint)
		if err != nil {
			return nil, errors.New("endpoint must be a valid URL")
		}
	}

	userAgent := fmt.Sprintf("%s/%s (%s/%s)",
		set.BuildInfo.Description, set.BuildInfo.Version, runtime.GOOS, runtime.GOARCH)

	// client construction is deferred to start
	return &exporter{
		config:    oCfg,
		logger:    set.Logger,
		userAgent: userAgent,
		settings:  set.TelemetrySettings,
	}, nil
}

// start actually creates the HTTP client. The client construction is deferred till this point as this
// is the only place we get hold of Extensions which are required to construct auth round tripper.
func (e *exporter) start(_ context.Context, host component.Host) error {
	client, err := e.config.HTTPClientSettings.ToClient(host, e.settings)
	if err != nil {
		return err
	}
	e.client = client
	return nil
}

func (e *exporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	tr := ptraceotlp.NewRequestFromTraces(td)
	request, err := tr.MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}

	return e.export(ctx, e.tracesURL, request)
}

func (e *exporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {
	tr := pmetricotlp.NewRequestFromMetrics(md)
	request, err := tr.MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}
	return e.export(ctx, e.metricsURL, request)
}

func (e *exporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	tr := plogotlp.NewRequestFromLogs(ld)
	request, err := tr.MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}

	return e.export(ctx, e.logsURL, request)
}

func (e *exporter) export(ctx context.Context, url string, request []byte) error {
	e.logger.Debug("Preparing to make HTTP request", zap.String("url", url))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(request))
	if err != nil {
		return consumererror.NewPermanent(err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", e.userAgent)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make an HTTP request: %w", err)
	}

	defer func() {
		// Discard any remaining response body when we are done reading.
		io.CopyN(io.Discard, resp.Body, maxHTTPResponseReadBytes) // nolint:errcheck
		resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		// Request is successful.
		return nil
	}

	respStatus := readResponse(resp)

	// Format the error message. Use the status if it is present in the response.
	var formattedErr error
	if respStatus != nil {
		formattedErr = fmt.Errorf(
			"error exporting items, request to %s responded with HTTP Status Code %d, Message=%s, Details=%v",
			url, resp.StatusCode, respStatus.Message, respStatus.Details)
	} else {
		formattedErr = fmt.Errorf(
			"error exporting items, request to %s responded with HTTP Status Code %d",
			url, resp.StatusCode)
	}

	// Check if the server is overwhelmed.
	// See spec https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#throttling-1
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		// Fallback to 0 if the Retry-After header is not present. This will trigger the
		// default backoff policy by our caller (retry handler).
		retryAfter := 0
		if val := resp.Header.Get(headerRetryAfter); val != "" {
			if seconds, err2 := strconv.Atoi(val); err2 == nil {
				retryAfter = seconds
			}
		}
		// Indicate to our caller to pause for the specified number of seconds.
		return exporterhelper.NewThrottleRetry(formattedErr, time.Duration(retryAfter)*time.Second)
	}

	if isPermanentClientFailure(resp.StatusCode) {
		// Do not retry; report the failure as permanent if the server thinks the request is malformed.
		return consumererror.NewPermanent(formattedErr)
	}

	// All other errors are retryable, so don't wrap them in consumererror.NewPermanent().
	return formattedErr
}

// Does the 'code' indicate a permanent error
func isPermanentClientFailure(code int) bool {
	switch code {
	case http.StatusBadRequest:
		return true
	case http.StatusPaymentRequired:
		// 402 - payment required typically means that an auth token isn't valid anymore and as such, we deem it as permanent
		return true
	case http.StatusRequestEntityTooLarge:
		return true
	case http.StatusRequestURITooLong:
		return true
	case http.StatusRequestHeaderFieldsTooLarge:
		return true
	case http.StatusNotFound:
		return true
	case http.StatusMethodNotAllowed:
		return true
	default:
		return false
	}
}

// Read the response and decode the status.Status from the body.
// Returns nil if the response is empty or cannot be decoded.
func readResponse(resp *http.Response) *status.Status {
	var respStatus *status.Status
	if resp.StatusCode >= 400 && resp.StatusCode <= 599 {
		// Request failed. Read the body. OTLP spec says:
		// "Response body for all HTTP 4xx and HTTP 5xx responses MUST be a
		// Protobuf-encoded Status message that describes the problem."
		maxRead := resp.ContentLength
		if maxRead == -1 || maxRead > maxHTTPResponseReadBytes {
			maxRead = maxHTTPResponseReadBytes
		}
		respBytes := make([]byte, maxRead)
		n, err := io.ReadFull(resp.Body, respBytes)
		if err == nil && n > 0 {
			// Decode it as Status struct. See https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#failures
			respStatus = &status.Status{}
			err = proto.Unmarshal(respBytes, respStatus)
			if err != nil {
				respStatus = nil
			}
		}
	}

	return respStatus
}
