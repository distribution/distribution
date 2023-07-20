// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/newrelic/go-agent/v3/internal"
	"github.com/newrelic/go-agent/v3/internal/logger"
)

const (
	// procotolVersion is the protocol version used to communicate with NR
	// backend.
	procotolVersion = 17

	userAgentPrefix = "NewRelic-Go-Agent/"

	// Methods used in collector communication.
	cmdPreconnect   = "preconnect"
	cmdConnect      = "connect"
	cmdMetrics      = "metric_data"
	cmdCustomEvents = "custom_event_data"
	cmdLogEvents    = "log_event_data"
	cmdTxnEvents    = "analytic_event_data"
	cmdErrorEvents  = "error_event_data"
	cmdErrorData    = "error_data"
	cmdTxnTraces    = "transaction_sample_data"
	cmdSlowSQLs     = "sql_trace_data"
	cmdSpanEvents   = "span_event_data"
)

// rpmCmd contains fields specific to an individual call made to RPM.
type rpmCmd struct {
	Name              string
	Collector         string
	RunID             string
	Data              []byte
	RequestHeadersMap map[string]string
	MaxPayloadSize    int
}

// rpmControls contains fields which will be the same for all calls made
// by the same application.
type rpmControls struct {
	License        string
	Client         *http.Client
	Logger         logger.Logger
	GzipWriterPool *sync.Pool
}

// rpmResponse contains a NR endpoint response.
//
// Agent Behavior Summary:
//
// on connect/preconnect:
//     410 means shutdown
//     200, 202 mean success (start run)
//     all other response codes and errors mean try after backoff
//
// on harvest:
//     410 means shutdown
//     401, 409 mean restart run
//     408, 429, 500, 503 mean save data for next harvest
//     all other response codes and errors discard the data and continue the current harvest
type rpmResponse struct {
	statusCode int
	body       []byte
	// Err indicates whether or not the call was successful: newRPMResponse
	// should be used to avoid mismatch between statusCode and Err.
	Err                      error
	disconnectSecurityPolicy bool
	// forceSaveHarvestData overrides the status code and forces a save of data
	forceSaveHarvestData bool
}

func newRPMResponse(statusCode int) rpmResponse {
	var err error
	if statusCode != 200 && statusCode != 202 {
		err = fmt.Errorf("response code: %d", statusCode)
	}
	return rpmResponse{statusCode: statusCode, Err: err}
}

// IsDisconnect indicates that the agent should disconnect.
func (resp rpmResponse) IsDisconnect() bool {
	return resp.statusCode == 410 || resp.disconnectSecurityPolicy
}

// IsRestartException indicates that the agent should restart.
func (resp rpmResponse) IsRestartException() bool {
	return resp.statusCode == 401 ||
		resp.statusCode == 409
}

// ShouldSaveHarvestData indicates that the agent should save the data and try
// to send it in the next harvest.
func (resp rpmResponse) ShouldSaveHarvestData() bool {
	if resp.forceSaveHarvestData {
		return true
	}
	switch resp.statusCode {
	case 408, 429, 500, 503:
		return true
	default:
		return false
	}
}

func rpmURL(cmd rpmCmd, cs rpmControls) string {
	var u url.URL

	u.Host = cmd.Collector
	u.Path = "agent_listener/invoke_raw_method"
	u.Scheme = "https"

	query := url.Values{}
	query.Set("marshal_format", "json")
	query.Set("protocol_version", strconv.Itoa(procotolVersion))
	query.Set("method", cmd.Name)
	query.Set("license_key", cs.License)

	if len(cmd.RunID) > 0 {
		query.Set("run_id", cmd.RunID)
	}

	u.RawQuery = query.Encode()
	return u.String()
}

func compress(b []byte, gzipWriterPool *sync.Pool) (*bytes.Buffer, error) {
	w := gzipWriterPool.Get().(*gzip.Writer)
	defer gzipWriterPool.Put(w)

	var buf bytes.Buffer
	w.Reset(&buf)
	_, err := w.Write(b)
	w.Close()

	if nil != err {
		return nil, err
	}

	return &buf, nil
}

func collectorRequestInternal(url string, cmd rpmCmd, cs rpmControls) rpmResponse {
	compressed, err := compress(cmd.Data, cs.GzipWriterPool)
	if nil != err {
		return rpmResponse{Err: err}
	}

	if l := compressed.Len(); l > cmd.MaxPayloadSize {
		return rpmResponse{Err: fmt.Errorf("Payload size for %s too large: %d greater than %d", cmd.Name, l, cmd.MaxPayloadSize)}
	}

	req, err := http.NewRequest("POST", url, compressed)
	if nil != err {
		return rpmResponse{Err: err}
	}

	req.Header.Add("Accept-Encoding", "identity, deflate")
	req.Header.Add("Content-Type", "application/octet-stream")
	req.Header.Add("User-Agent", userAgentPrefix+Version)
	req.Header.Add("Content-Encoding", "gzip")
	for k, v := range cmd.RequestHeadersMap {
		req.Header.Add(k, v)
	}

	resp, err := cs.Client.Do(req)
	if err != nil {
		return rpmResponse{
			forceSaveHarvestData: true,
			Err:                  err,
		}
	}

	defer resp.Body.Close()

	r := newRPMResponse(resp.StatusCode)

	// Read the entire response, rather than using resp.Body as input to json.NewDecoder to
	// avoid the issue described here:
	// https://github.com/google/go-github/pull/317
	// https://ahmetalpbalkan.com/blog/golang-json-decoder-pitfalls/
	// Also, collector JSON responses are expected to be quite small.
	body, err := ioutil.ReadAll(resp.Body)
	if nil == r.Err {
		r.Err = err
	}
	r.body = body

	return r
}

// collectorRequest makes a request to New Relic.
func collectorRequest(cmd rpmCmd, cs rpmControls) rpmResponse {
	url := rpmURL(cmd, cs)
	urlWithoutLicense := removeLicenseFromURL(url)

	if cs.Logger.DebugEnabled() {
		cs.Logger.Debug("rpm request", map[string]interface{}{
			"command": cmd.Name,
			"url":     urlWithoutLicense,
			"payload": jsonString(cmd.Data),
		})
	}

	resp := collectorRequestInternal(url, cmd, cs)

	if cs.Logger.DebugEnabled() {
		if err := resp.Err; err != nil {
			cs.Logger.Debug("rpm failure", map[string]interface{}{
				"command":  cmd.Name,
				"url":      urlWithoutLicense,
				"response": string(resp.body), // Body might not be JSON on failure.
				"error":    err.Error(),
			})
		} else {
			cs.Logger.Debug("rpm response", map[string]interface{}{
				"command":  cmd.Name,
				"url":      urlWithoutLicense,
				"response": jsonString(resp.body),
			})
		}
	}

	return resp
}

func removeLicenseFromURL(u string) string {
	rawURL, err := url.Parse(u)
	if err != nil {
		return ""
	}

	query := rawURL.Query()
	licenseKey := query.Get("license_key")

	// License key length has already been checked, but doing another
	// conservative check here.
	if n := len(licenseKey); n > 4 {
		query.Set("license_key", string(licenseKey[0:2]+".."+licenseKey[n-2:]))
	}
	rawURL.RawQuery = query.Encode()
	return rawURL.String()
}

type preconnectRequest struct {
	SecurityPoliciesToken string `json:"security_policies_token,omitempty"`
	HighSecurity          bool   `json:"high_security"`
}

var (
	errMissingAgentRunID = errors.New("connect reply missing agent run id")
)

// connectAttempt tries to connect an application.
func connectAttempt(config config, cs rpmControls) (*internal.ConnectReply, rpmResponse) {
	preconnectData, err := json.Marshal([]preconnectRequest{{
		SecurityPoliciesToken: config.SecurityPoliciesToken,
		HighSecurity:          config.HighSecurity,
	}})
	if nil != err {
		return nil, rpmResponse{Err: fmt.Errorf("unable to marshal preconnect data: %v", err)}
	}

	call := rpmCmd{
		Name:           cmdPreconnect,
		Collector:      config.preconnectHost(),
		Data:           preconnectData,
		MaxPayloadSize: internal.MaxPayloadSizeInBytes,
	}

	resp := collectorRequest(call, cs)
	if nil != resp.Err {
		return nil, resp
	}

	var preconnect struct {
		Preconnect internal.PreconnectReply `json:"return_value"`
	}
	err = json.Unmarshal(resp.body, &preconnect)
	if nil != err {
		// Certain security policy errors must be treated as a disconnect.
		return nil, rpmResponse{
			Err:                      fmt.Errorf("unable to process preconnect reply: %v", err),
			disconnectSecurityPolicy: internal.IsDisconnectSecurityPolicyError(err),
		}
	}

	js, err := config.createConnectJSON(preconnect.Preconnect.SecurityPolicies.PointerIfPopulated())
	if nil != err {
		return nil, rpmResponse{Err: fmt.Errorf("unable to create connect data: %v", err)}
	}

	call.Collector = preconnect.Preconnect.Collector
	call.Data = js
	call.Name = cmdConnect

	resp = collectorRequest(call, cs)
	if nil != resp.Err {
		return nil, resp
	}

	reply, err := internal.UnmarshalConnectReply(resp.body, preconnect.Preconnect)
	if nil != err {
		return nil, rpmResponse{Err: err}
	}

	// Note:  This should never happen.  It would mean the collector
	// response is malformed.  This exists merely as extra defensiveness.
	if "" == reply.RunID {
		return nil, rpmResponse{Err: errMissingAgentRunID}
	}

	return reply, resp
}
