// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
	"github.com/newrelic/go-agent/v3/internal/jsonx"
)

type distTraceVersion [2]int

func (v distTraceVersion) major() int { return v[0] }
func (v distTraceVersion) minor() int { return v[1] }

// WriteJSON implements the functionality to support writerField
// in our internal json builder. It appends the JSON representation
// of a distTraceVersion to the destination bytes.Buffer.
func (v distTraceVersion) WriteJSON(buf *bytes.Buffer) {
	jsonx.AppendIntArray(buf, int64(v[0]), int64(v[1]))
}

const (
	// callerTypeApp is the Type field's value for outbound payloads.
	callerTypeApp = "App"
	// callerTypeBrowser is the Type field's value for browser payloads
	callerTypeBrowser = "Browser"
	// callerTypeMobile is the Type field's value for mobile payloads
	callerTypeMobile = "Mobile"
)

var (
	currentDistTraceVersion = distTraceVersion([2]int{0 /* Major */, 1 /* Minor */})
	callerUnknown           = payloadCaller{Type: "Unknown", App: "Unknown", Account: "Unknown", TransportType: "Unknown"}
	traceParentRegex        = regexp.MustCompile(`^([a-f0-9]{2})-` + // version
		`([a-f0-9]{32})-` + // traceId
		`([a-f0-9]{16})-` + // parentId
		`([a-f0-9]{2})(-.*)?$`) // flags
)

// timestampMillis allows raw payloads to use exact times, and marshalled
// payloads to use times in millis.
type timestampMillis time.Time

func (tm *timestampMillis) UnmarshalJSON(data []byte) error {
	var millis uint64
	if err := json.Unmarshal(data, &millis); nil != err {
		return err
	}
	*tm = timestampMillis(timeFromUnixMilliseconds(millis))
	return nil
}

func (tm timestampMillis) MarshalJSON() ([]byte, error) {
	return json.Marshal(timeToUnixMilliseconds(tm.Time()))
}

// WriteJSON implements the functionality to support writerField
// in our internal json builder. It appends the JSON representation
// of a timestampMillis value to the destination bytes.Buffer.
func (tm timestampMillis) WriteJSON(buf *bytes.Buffer) {
	jsonx.AppendUint(buf, timeToUnixMilliseconds(tm.Time()))
}

func (tm timestampMillis) Time() time.Time  { return time.Time(tm) }
func (tm *timestampMillis) Set(t time.Time) { *tm = timestampMillis(t) }

func (tm timestampMillis) unixMillisecondsString() string {
	ms := timeToUnixMilliseconds(tm.Time())
	return strconv.FormatUint(ms, 10)
}

// payload is the distributed tracing payload.
type payload struct {
	Type          string   `json:"ty"`
	App           string   `json:"ap"`
	Account       string   `json:"ac"`
	TransactionID string   `json:"tx,omitempty"`
	ID            string   `json:"id,omitempty"`
	TracedID      string   `json:"tr"`
	Priority      priority `json:"pr"`
	// This is a *bool instead of a normal bool so we can tell the different between unset and false.
	Sampled              *bool           `json:"sa"`
	Timestamp            timestampMillis `json:"ti"`
	TransportDuration    time.Duration   `json:"-"`
	TrustedParentID      string          `json:"-"`
	TracingVendors       string          `json:"-"`
	HasNewRelicTraceInfo bool            `json:"-"`
	TrustedAccountKey    string          `json:"tk,omitempty"`
	NonTrustedTraceState string          `json:"-"`
	OriginalTraceState   string          `json:"-"`
}

// WriteJSON implements the functionality to support writerField
// in our internal json builder. It appends the JSON representation
// of a payload struct to the destination bytes.Buffer.
func (p payload) WriteJSON(buf *bytes.Buffer) {
	buf.WriteByte('{')
	w := jsonFieldsWriter{buf: buf}
	w.stringField("ty", p.Type)
	w.stringField("ap", p.App)
	w.stringField("ac", p.Account)
	if p.TransactionID != "" {
		w.stringField("tx", p.TransactionID)
	}
	if p.ID != "" {
		w.stringField("id", p.ID)
	}
	w.stringField("tr", p.TracedID)
	w.float32Field("pr", float32(p.Priority))

	if p.Sampled == nil {
		w.addKey("sa")
		w.buf.WriteString("null")
	} else {
		w.boolField("sa", *p.Sampled)
	}
	w.writerField("ti", p.Timestamp)
	if p.TrustedAccountKey != "" {
		w.stringField("tk", p.TrustedAccountKey)
	}
	buf.WriteByte('}')
}

type payloadCaller struct {
	TransportType string
	Type          string
	App           string
	Account       string
}

var (
	errPayloadMissingGUIDTxnID = errors.New("payload is missing both guid/id and TransactionId/tx")
	errPayloadMissingType      = errors.New("payload is missing Type/ty")
	errPayloadMissingAccount   = errors.New("payload is missing Account/ac")
	errPayloadMissingApp       = errors.New("payload is missing App/ap")
	errPayloadMissingTraceID   = errors.New("payload is missing TracedID/tr")
	errPayloadMissingTimestamp = errors.New("payload is missing Timestamp/ti")
	errPayloadMissingVersion   = errors.New("payload is missing Version/v")
)

// IsValid IsValidNewRelicData the payload data by looking for missing fields.
// Returns an error if there's a problem, nil if everything's fine
func (p payload) validateNewRelicData() error {

	// If a payload is missing both `guid` and `transactionId` is received,
	// a ParseException supportability metric should be generated.
	if p.TransactionID == "" && p.ID == "" {
		return errPayloadMissingGUIDTxnID
	}

	if p.Type == "" {
		return errPayloadMissingType
	}

	if p.Account == "" {
		return errPayloadMissingAccount
	}

	if p.App == "" {
		return errPayloadMissingApp
	}

	if p.TracedID == "" {
		return errPayloadMissingTraceID
	}

	if p.Timestamp.Time().IsZero() || p.Timestamp.Time().Unix() == 0 {
		return errPayloadMissingTimestamp
	}

	return nil
}

const payloadJSONStartingSizeEstimate = 256

func (p payload) text(v distTraceVersion) []byte {
	// TrustedAccountKey should only be attached to the outbound payload if its value differs
	// from the Account field.
	if p.TrustedAccountKey == p.Account {
		p.TrustedAccountKey = ""
	}

	js := bytes.NewBuffer(make([]byte, 0, payloadJSONStartingSizeEstimate))
	w := jsonFieldsWriter{
		buf: js,
	}
	js.WriteByte('{')
	w.writerField("v", v)
	w.writerField("d", p)
	js.WriteByte('}')

	return js.Bytes()
}

// NRText implements newrelic.DistributedTracePayload.
func (p payload) NRText() string {
	t := p.text(currentDistTraceVersion)
	return string(t)
}

// NRHTTPSafe implements newrelic.DistributedTracePayload.
func (p payload) NRHTTPSafe() string {
	t := p.text(currentDistTraceVersion)
	return base64.StdEncoding.EncodeToString(t)
}

var (
	typeMap = map[string]string{
		callerTypeApp:     "0",
		callerTypeBrowser: "1",
		callerTypeMobile:  "2",
	}
	typeMapReverse = func() map[string]string {
		reversed := make(map[string]string)
		for k, v := range typeMap {
			reversed[v] = k
		}
		return reversed
	}()
)

const (
	w3cVersion        = "00"
	traceStateVersion = "0"
)

// W3CTraceParent returns the W3C TraceParent header for this payload
func (p payload) W3CTraceParent() string {
	var flags string
	if p.isSampled() {
		flags = "01"
	} else {
		flags = "00"
	}
	traceID := strings.ToLower(p.TracedID)
	if idLen := len(traceID); idLen < internal.TraceIDHexStringLen {
		traceID = strings.Repeat("0", internal.TraceIDHexStringLen-idLen) + traceID
	} else if idLen > internal.TraceIDHexStringLen {
		traceID = traceID[idLen-internal.TraceIDHexStringLen:]
	}
	return w3cVersion + "-" + traceID + "-" + p.ID + "-" + flags
}

// W3CTraceState returns the W3C TraceState header for this payload
func (p payload) W3CTraceState() string {
	var flags string

	if p.isSampled() {
		flags = "1"
	} else {
		flags = "0"
	}
	state := p.TrustedAccountKey + "@nr=" +
		traceStateVersion + "-" +
		typeMap[p.Type] + "-" +
		p.Account + "-" +
		p.App + "-" +
		p.ID + "-" +
		p.TransactionID + "-" +
		flags + "-" +
		p.Priority.traceStateFormat() + "-" +
		p.Timestamp.unixMillisecondsString()
	if p.NonTrustedTraceState != "" {
		state += "," + p.NonTrustedTraceState
	}
	return state
}

var (
	trueVal  = true
	falseVal = false
	boolPtrs = map[bool]*bool{
		true:  &trueVal,
		false: &falseVal,
	}
)

// SetSampled lets us set a value for our *bool,
// which we can't do directly since a pointer
// needs something to point at.
func (p *payload) SetSampled(sampled bool) {
	p.Sampled = boolPtrs[sampled]
}

func (p payload) isSampled() bool {
	return p.Sampled != nil && *p.Sampled
}

// acceptPayload parses the inbound distributed tracing payload.
func acceptPayload(hdrs http.Header, trustedAccountKey string, support *distributedTracingSupport) (*payload, error) {
	if hdrs.Get(DistributedTraceW3CTraceParentHeader) != "" {
		return processW3CHeaders(hdrs, trustedAccountKey, support)
	}
	return processNRDTString(hdrs.Get(DistributedTraceNewRelicHeader), support)
}

func processNRDTString(str string, support *distributedTracingSupport) (*payload, error) {
	if str == "" {
		return nil, nil
	}
	var decoded []byte
	if str[0] == '{' {
		decoded = []byte(str)
	} else {
		var err error
		decoded, err = base64.StdEncoding.DecodeString(str)
		if err != nil {
			support.AcceptPayloadParseException = true
			return nil, fmt.Errorf("unable to decode payload: %v", err)
		}
	}
	envelope := struct {
		Version distTraceVersion `json:"v"`
		Data    json.RawMessage  `json:"d"`
	}{}
	if err := json.Unmarshal(decoded, &envelope); err != nil {
		support.AcceptPayloadParseException = true
		return nil, fmt.Errorf("unable to unmarshal payload: %v", err)
	}

	if envelope.Version.major() == 0 && envelope.Version.minor() == 0 {
		support.AcceptPayloadParseException = true
		return nil, errPayloadMissingVersion
	}

	if envelope.Version.major() > currentDistTraceVersion.major() {
		support.AcceptPayloadIgnoredVersion = true
		return nil, fmt.Errorf("unsupported major version number %v",
			envelope.Version.major())
	}
	payload := new(payload)
	if err := json.Unmarshal(envelope.Data, payload); err != nil {
		support.AcceptPayloadParseException = true
		return nil, fmt.Errorf("unable to unmarshal payload data: %v", err)
	}

	payload.HasNewRelicTraceInfo = true
	if err := payload.validateNewRelicData(); err != nil {
		support.AcceptPayloadParseException = true
		return nil, err
	}
	support.AcceptPayloadSuccess = true
	return payload, nil
}

func processW3CHeaders(hdrs http.Header, trustedAccountKey string, support *distributedTracingSupport) (*payload, error) {
	p, err := processTraceParent(hdrs)
	if err != nil {
		support.TraceContextParentParseException = true
		return nil, err
	}
	err = processTraceState(hdrs, trustedAccountKey, p)
	if err != nil {
		if err == errInvalidNRTraceState {
			support.TraceContextStateInvalidNrEntry = true
		} else {
			support.TraceContextStateNoNrEntry = true
		}
	}
	support.TraceContextAcceptSuccess = true
	return p, nil
}

var (
	errTooManyHdrs         = errors.New("too many TraceParent headers")
	errNumEntries          = errors.New("invalid number of TraceParent entries")
	errInvalidTraceID      = errors.New("invalid TraceParent trace ID")
	errInvalidParentID     = errors.New("invalid TraceParent parent ID")
	errInvalidFlags        = errors.New("invalid TraceParent flags for this version")
	errInvalidNRTraceState = errors.New("invalid NR entry in trace state")
	errMissingTrustedNR    = errors.New("no trusted NR entry found in trace state")
)

func processTraceParent(hdrs http.Header) (*payload, error) {
	traceParents := hdrs[DistributedTraceW3CTraceParentHeader]
	if len(traceParents) > 1 {
		return nil, errTooManyHdrs
	}
	subMatches := traceParentRegex.FindStringSubmatch(traceParents[0])

	if subMatches == nil || len(subMatches) != 6 {
		return nil, errNumEntries
	}
	if !validateVersionAndFlags(subMatches) {
		return nil, errInvalidFlags
	}

	p := new(payload)
	p.TracedID = subMatches[2]
	if p.TracedID == "00000000000000000000000000000000" {
		return nil, errInvalidTraceID
	}
	p.ID = subMatches[3]
	if p.ID == "0000000000000000" {
		return nil, errInvalidParentID
	}

	return p, nil
}

func validateVersionAndFlags(subMatches []string) bool {
	if subMatches[1] == w3cVersion {
		if subMatches[5] != "" {
			return false
		}
	}
	// Invalid version: https://w3c.github.io/trace-context/#version
	if subMatches[1] == "ff" {
		return false
	}
	return true
}

func processTraceState(hdrs http.Header, trustedAccountKey string, p *payload) error {
	traceStates := hdrs[DistributedTraceW3CTraceStateHeader]
	fullTraceState := strings.Join(traceStates, ",")
	p.OriginalTraceState = fullTraceState

	var trustedVal string
	p.TracingVendors, p.NonTrustedTraceState, trustedVal = parseTraceState(fullTraceState, trustedAccountKey)
	if trustedVal == "" {
		return errMissingTrustedNR
	}

	matches := strings.Split(trustedVal, "-")
	if len(matches) < 9 {
		return errInvalidNRTraceState
	}

	// Required Fields:
	version := matches[0]
	parentType := typeMapReverse[matches[1]]
	account := matches[2]
	app := matches[3]
	timestamp, err := strconv.ParseUint(matches[8], 10, 64)

	if err != nil || version == "" || parentType == "" || account == "" || app == "" {
		return errInvalidNRTraceState
	}

	p.TrustedAccountKey = trustedAccountKey
	p.Type = parentType
	p.Account = account
	p.App = app
	p.TrustedParentID = matches[4]
	p.TransactionID = matches[5]

	// If sampled isn't "1" or "0", leave it unset
	if matches[6] == "1" {
		p.SetSampled(true)
	} else if matches[6] == "0" {
		p.SetSampled(false)
	}
	pty, err := strconv.ParseFloat(matches[7], 32)
	if nil == err {
		p.Priority = priority(pty)
	}
	p.Timestamp = timestampMillis(timeFromUnixMilliseconds(timestamp))
	p.HasNewRelicTraceInfo = true
	return nil
}

func parseTraceState(fullState, trustedAccountKey string) (nonTrustedVendors string, nonTrustedState string, trustedEntryValue string) {
	trustedKey := trustedAccountKey + "@nr"
	pairs := strings.Split(fullState, ",")
	vendors := make([]string, 0, len(pairs))
	states := make([]string, 0, len(pairs))
	for _, entry := range pairs {
		entry = strings.TrimSpace(entry)
		m := strings.Split(entry, "=")
		if len(m) != 2 {
			continue
		}
		if key, val := m[0], m[1]; key == trustedKey {
			trustedEntryValue = val
		} else {
			vendors = append(vendors, key)
			states = append(states, entry)
		}
	}
	nonTrustedVendors = strings.Join(vendors, ",")
	nonTrustedState = strings.Join(states, ",")
	return
}
