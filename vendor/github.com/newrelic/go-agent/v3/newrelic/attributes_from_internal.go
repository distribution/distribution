// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	// query parameters only appear in segments, not span events, but is
	// listed as span attributes to simplify code. It is not listed in the
	// public attributes.go file for this reason to prevent confusion.
	spanAttributeQueryParameters = "query_parameters"
)

var (
	usualDests  = destAll &^ destBrowser
	tracesDests = destTxnTrace | destError
	//
	// To add an agent attribute, add it to the public constants in
	// attributes.go and add its default destinations here.
	//
	agentAttributeDefaultDests = map[string]destinationSet{
		AttributeHostDisplayName:            usualDests,
		AttributeRequestMethod:              usualDests,
		AttributeRequestAccept:              usualDests,
		AttributeRequestContentType:         usualDests,
		AttributeRequestContentLength:       usualDests,
		AttributeRequestHost:                usualDests,
		AttributeRequestUserAgent:           tracesDests,
		AttributeRequestUserAgentDeprecated: tracesDests,
		AttributeRequestReferer:             tracesDests,
		AttributeRequestURI:                 usualDests,
		AttributeResponseContentType:        usualDests,
		AttributeResponseContentLength:      usualDests,
		AttributeResponseCode:               usualDests,
		AttributeResponseCodeDeprecated:     usualDests,
		AttributeAWSRequestID:               usualDests,
		AttributeAWSLambdaARN:               usualDests,
		AttributeAWSLambdaColdStart:         usualDests,
		AttributeAWSLambdaEventSourceARN:    usualDests,
		AttributeMessageRoutingKey:          usualDests,
		AttributeMessageQueueName:           usualDests,
		AttributeMessageExchangeType:        destNone,
		AttributeMessageReplyTo:             destNone,
		AttributeMessageCorrelationID:       destNone,
		AttributeCodeFunction:               usualDests,
		AttributeCodeNamespace:              usualDests,
		AttributeCodeFilepath:               usualDests,
		AttributeCodeLineno:                 usualDests,
		AttributeUserID:                     usualDests,

		// Span specific attributes
		SpanAttributeDBStatement:             usualDests,
		SpanAttributeDBInstance:              usualDests,
		SpanAttributeDBCollection:            usualDests,
		SpanAttributePeerAddress:             usualDests,
		SpanAttributePeerHostname:            usualDests,
		SpanAttributeHTTPURL:                 usualDests,
		SpanAttributeHTTPMethod:              usualDests,
		spanAttributeQueryParameters:         usualDests,
		SpanAttributeAWSOperation:            usualDests,
		SpanAttributeAWSRegion:               usualDests,
		SpanAttributeErrorClass:              usualDests,
		SpanAttributeErrorMessage:            usualDests,
		SpanAttributeParentType:              usualDests,
		SpanAttributeParentApp:               usualDests,
		SpanAttributeParentAccount:           usualDests,
		SpanAttributeParentTransportDuration: usualDests,
		SpanAttributeParentTransportType:     usualDests,
	}
)

// https://source.datanerd.us/agents/agent-specs/blob/master/Agent-Attributes-PORTED.md

type destinationSet int

const (
	destTxnEvent destinationSet = 1 << iota
	destError
	destTxnTrace
	destBrowser
	destSpan
	destSegment
)

const (
	destNone destinationSet = 0
	// destAll contains all destinations.
	destAll destinationSet = destTxnEvent | destTxnTrace | destError | destBrowser | destSpan | destSegment
)

const (
	attributeWildcardSuffix = '*'
)

type attributeModifier struct {
	match string // This will not contain a trailing '*'.
	includeExclude
}

type byMatch []*attributeModifier

func (m byMatch) Len() int           { return len(m) }
func (m byMatch) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m byMatch) Less(i, j int) bool { return m[i].match < m[j].match }

// attributeConfig is created at connect and shared between all transactions.
type attributeConfig struct {
	disabledDestinations destinationSet
	exactMatchModifiers  map[string]*attributeModifier
	// Once attributeConfig is constructed, wildcardModifiers is sorted in
	// lexicographical order.  Modifiers appearing later have precedence
	// over modifiers appearing earlier.
	wildcardModifiers []*attributeModifier
	agentDests        map[string]destinationSet
}

type includeExclude struct {
	include destinationSet
	exclude destinationSet
}

func modifierApply(m *attributeModifier, d destinationSet) destinationSet {
	// Include before exclude, since exclude has priority.
	d |= m.include
	d &^= m.exclude
	return d
}

func applyAttributeConfig(c *attributeConfig, key string, d destinationSet) destinationSet {
	// Important: The wildcard modifiers must be applied before the exact
	// match modifiers, and the slice must be iterated in a forward
	// direction.
	for _, m := range c.wildcardModifiers {
		if strings.HasPrefix(key, m.match) {
			d = modifierApply(m, d)
		}
	}

	if m, ok := c.exactMatchModifiers[key]; ok {
		d = modifierApply(m, d)
	}

	d &^= c.disabledDestinations

	return d
}

func addModifier(c *attributeConfig, match string, d includeExclude) {
	if "" == match {
		return
	}
	exactMatch := true
	if attributeWildcardSuffix == match[len(match)-1] {
		exactMatch = false
		match = match[0 : len(match)-1]
	}
	mod := &attributeModifier{
		match:          match,
		includeExclude: d,
	}

	if exactMatch {
		if m, ok := c.exactMatchModifiers[mod.match]; ok {
			m.include |= mod.include
			m.exclude |= mod.exclude
		} else {
			c.exactMatchModifiers[mod.match] = mod
		}
	} else {
		for _, m := range c.wildcardModifiers {
			// Important: Duplicate entries for the same match
			// string would not work because exclude needs
			// precedence over include.
			if m.match == mod.match {
				m.include |= mod.include
				m.exclude |= mod.exclude
				return
			}
		}
		c.wildcardModifiers = append(c.wildcardModifiers, mod)
	}
}

func processDest(c *attributeConfig, includeEnabled bool, dc *AttributeDestinationConfig, d destinationSet) {
	if !dc.Enabled {
		c.disabledDestinations |= d
	}
	if includeEnabled {
		for _, match := range dc.Include {
			addModifier(c, match, includeExclude{include: d})
		}
	}
	for _, match := range dc.Exclude {
		addModifier(c, match, includeExclude{exclude: d})
	}
}

// createAttributeConfig creates a new attributeConfig.
func createAttributeConfig(input config, includeEnabled bool) *attributeConfig {
	c := &attributeConfig{
		exactMatchModifiers: make(map[string]*attributeModifier),
		wildcardModifiers:   make([]*attributeModifier, 0, 64),
	}

	processDest(c, includeEnabled, &input.Attributes, destAll)
	processDest(c, includeEnabled, &input.ErrorCollector.Attributes, destError)
	processDest(c, includeEnabled, &input.TransactionEvents.Attributes, destTxnEvent)
	processDest(c, includeEnabled, &input.TransactionTracer.Attributes, destTxnTrace)
	processDest(c, includeEnabled, &input.BrowserMonitoring.Attributes, destBrowser)
	processDest(c, includeEnabled, &input.SpanEvents.Attributes, destSpan)
	processDest(c, includeEnabled, &input.TransactionTracer.Segments.Attributes, destSegment)

	sort.Sort(byMatch(c.wildcardModifiers))

	c.agentDests = make(map[string]destinationSet)
	for name, dest := range agentAttributeDefaultDests {
		c.agentDests[name] = applyAttributeConfig(c, name, dest)
	}

	return c
}

type userAttribute struct {
	value interface{}
	dests destinationSet
}

type agentAttributeValue struct {
	stringVal string
	otherVal  interface{}
}

type agentAttributes map[string]agentAttributeValue

func (a *attributes) filterSpanAttributes(s map[string]jsonWriter, d destinationSet) map[string]jsonWriter {
	if nil != a {
		for key := range s {
			if a.config.agentDests[key]&d == 0 {
				delete(s, key)
			}
		}
	}
	return s
}

// GetAgentValue is used to access agent attributes.  This function returns ("",
// nil) if the attribute doesn't exist or it doesn't match the destinations
// provided.
func (a *attributes) GetAgentValue(id string, d destinationSet) (string, interface{}) {
	if nil == a || 0 == a.config.agentDests[id]&d {
		return "", nil
	}
	v, _ := a.Agent[id]
	return v.stringVal, v.otherVal
}

// Add is used to add agent attributes.  Only one of stringVal and
// otherVal should be populated.  Since most agent attribute values are strings,
// stringVal exists to avoid allocations.
func (attr agentAttributes) Add(id string, stringVal string, otherVal interface{}) {
	if stringVal != "" || otherVal != nil {
		attr[id] = agentAttributeValue{
			stringVal: truncateStringValueIfLong(stringVal),
			otherVal:  otherVal,
		}
	}
}

// Remove is used to remove agent attributes.
// It is not an error if the attribute wasn't present to begin with.
func (attr agentAttributes) Remove(id string) {
	if _, ok := attr[id]; ok {
		delete(attr, id)
	}
}

// attributes are key value pairs attached to the various collected data types.
type attributes struct {
	config *attributeConfig
	user   map[string]userAttribute
	Agent  agentAttributes
}

// newAttributes creates a new Attributes.
func newAttributes(config *attributeConfig) *attributes {
	return &attributes{
		config: config,
		Agent:  make(agentAttributes),
	}
}

// errInvalidAttributeType is returned when the value is not valid.
type errInvalidAttributeType struct {
	key string
	val interface{}
}

func (e errInvalidAttributeType) Error() string {
	return fmt.Sprintf("attribute '%s' value of type %T is invalid", e.key, e.val)
}

type invalidAttributeKeyErr struct{ key string }

func (e invalidAttributeKeyErr) Error() string {
	return fmt.Sprintf("attribute key '%.32s...' exceeds length limit %d",
		e.key, attributeKeyLengthLimit)
}

type userAttributeLimitErr struct{ key string }

func (e userAttributeLimitErr) Error() string {
	return fmt.Sprintf("attribute '%s' discarded: limit of %d reached", e.key,
		attributeUserLimit)
}

type invalidFloatAttrValue struct {
	key string
	val float64
}

func (e invalidFloatAttrValue) Error() string {
	return fmt.Sprintf("attribute '%s' of type float contains an invalid value: %f", e.key, e.val)

}

func truncateStringValueIfLong(val string) string {
	if len(val) > attributeValueLengthLimit {
		return stringLengthByteLimit(val, attributeValueLengthLimit)
	}
	return val
}

func truncateStringMessageIfLong(message string) string {
	if len(message) > errorEventMessageLengthLimit {
		return stringLengthByteLimit(message, errorEventMessageLengthLimit)
	}
	return message
}

// validateUserAttribute validates a user attribute.
func validateUserAttribute(key string, val interface{}) (interface{}, error) {
	if str, ok := val.(string); ok {
		val = interface{}(truncateStringValueIfLong(str))
	}

	switch v := val.(type) {
	case string, bool,
		uint8, uint16, uint32, uint64, int8, int16, int32, int64,
		uint, int, uintptr:
	case float32:
		if err := validateFloat(float64(v), key); err != nil {
			return nil, err
		}
	case float64:
		if err := validateFloat(v, key); err != nil {
			return nil, err
		}
	default:
		return nil, errInvalidAttributeType{
			key: key,
			val: val,
		}
	}

	// Attributes whose keys are excessively long are dropped rather than
	// truncated to avoid worrying about the application of configuration to
	// truncated values or performing the truncation after configuration.
	if len(key) > attributeKeyLengthLimit {
		return nil, invalidAttributeKeyErr{key: key}
	}
	return val, nil
}

func validateFloat(v float64, key string) error {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return invalidFloatAttrValue{
			key: key,
			val: v,
		}
	}
	return nil
}

// addUserAttribute adds a user attribute.
func addUserAttribute(a *attributes, key string, val interface{}, d destinationSet) error {
	val, err := validateUserAttribute(key, val)
	if nil != err {
		return err
	}
	dests := applyAttributeConfig(a.config, key, d)
	if destNone == dests {
		return nil
	}
	if nil == a.user {
		a.user = make(map[string]userAttribute)
	}

	if _, exists := a.user[key]; !exists && len(a.user) >= attributeUserLimit {
		return userAttributeLimitErr{key}
	}

	// Note: Duplicates are overridden: last attribute in wins.
	a.user[key] = userAttribute{
		value: val,
		dests: dests,
	}
	return nil
}

func writeAttributeValueJSON(w *jsonFieldsWriter, key string, val interface{}) {
	switch v := val.(type) {
	case string:
		w.stringField(key, v)
	case bool:
		if v {
			w.rawField(key, `true`)
		} else {
			w.rawField(key, `false`)
		}
	case uint8:
		w.intField(key, int64(v))
	case uint16:
		w.intField(key, int64(v))
	case uint32:
		w.intField(key, int64(v))
	case uint64:
		w.intField(key, int64(v))
	case uint:
		w.intField(key, int64(v))
	case uintptr:
		w.intField(key, int64(v))
	case int8:
		w.intField(key, int64(v))
	case int16:
		w.intField(key, int64(v))
	case int32:
		w.intField(key, int64(v))
	case int64:
		w.intField(key, v)
	case int:
		w.intField(key, int64(v))
	case float32:
		w.floatField(key, float64(v))
	case float64:
		w.floatField(key, v)
	default:
		w.stringField(key, fmt.Sprintf("%T", v))
	}
}

func agentAttributesJSON(a *attributes, buf *bytes.Buffer, d destinationSet, additionalAttributes ...map[string]string) {
	if a == nil {
		buf.WriteString("{}")
		return
	}

	w := jsonFieldsWriter{buf: buf}
	buf.WriteByte('{')
	for id, val := range a.Agent {
		if a.config.agentDests[id]&d != 0 {
			if val.stringVal != "" {
				w.stringField(id, val.stringVal)
			} else {
				writeAttributeValueJSON(&w, id, val.otherVal)
			}
		}
	}

	// Add additional agent attributes to json
	for _, additionalAttribute := range additionalAttributes {
		for id, val := range additionalAttribute {
			w.stringField(id, val)
		}
	}

	buf.WriteByte('}')
}

func userAttributesJSON(a *attributes, buf *bytes.Buffer, d destinationSet, extraAttributes map[string]interface{}) {
	buf.WriteByte('{')
	if nil != a {
		w := jsonFieldsWriter{buf: buf}
		for key, val := range extraAttributes {
			outputDest := applyAttributeConfig(a.config, key, d)
			if outputDest&d != 0 {
				writeAttributeValueJSON(&w, key, val)
			}
		}
		for name, atr := range a.user {
			if atr.dests&d != 0 {
				if _, found := extraAttributes[name]; found {
					continue
				}
				writeAttributeValueJSON(&w, name, atr.value)
			}
		}
	}
	buf.WriteByte('}')
}

// userAttributesStringJSON is only used for testing.
func userAttributesStringJSON(a *attributes, d destinationSet, extraAttributes map[string]interface{}) string {
	estimate := len(a.user) * 128
	buf := bytes.NewBuffer(make([]byte, 0, estimate))
	userAttributesJSON(a, buf, d, extraAttributes)
	return buf.String()
}

// RequestAgentAttributes gathers agent attributes out of the request.
func requestAgentAttributes(a *attributes, method string, hdrs http.Header, u *url.URL, host string) {
	a.Agent.Add(AttributeRequestMethod, method, nil)

	if nil != u {
		a.Agent.Add(AttributeRequestURI, safeURL(u), nil)
	}

	if nil == hdrs {
		return
	}
	a.Agent.Add(AttributeRequestAccept, hdrs.Get("Accept"), nil)
	a.Agent.Add(AttributeRequestContentType, hdrs.Get("Content-Type"), nil)
	a.Agent.Add(AttributeRequestUserAgent, hdrs.Get("User-Agent"), nil)
	a.Agent.Add(AttributeRequestUserAgentDeprecated, hdrs.Get("User-Agent"), nil)
	a.Agent.Add(AttributeRequestReferer, safeURLFromString(hdrs.Get("Referer")), nil)
	a.Agent.Add(AttributeRequestHost, host, nil)

	if l := getContentLengthFromHeader(hdrs); l >= 0 {
		a.Agent.Add(AttributeRequestContentLength, "", l)
	}
}

// responseHeaderAttributes gather agent attributes from the response headers.
func responseHeaderAttributes(a *attributes, h http.Header) {
	if nil == h {
		return
	}
	a.Agent.Add(AttributeResponseContentType, h.Get("Content-Type"), nil)

	if l := getContentLengthFromHeader(h); l >= 0 {
		a.Agent.Add(AttributeResponseContentLength, "", l)
	}
}

var (
	// statusCodeLookup avoids a strconv.Itoa call.
	statusCodeLookup = map[int]string{
		100: "100", 101: "101",
		200: "200", 201: "201", 202: "202", 203: "203", 204: "204", 205: "205", 206: "206",
		300: "300", 301: "301", 302: "302", 303: "303", 304: "304", 305: "305", 307: "307",
		400: "400", 401: "401", 402: "402", 403: "403", 404: "404", 405: "405", 406: "406",
		407: "407", 408: "408", 409: "409", 410: "410", 411: "411", 412: "412", 413: "413",
		414: "414", 415: "415", 416: "416", 417: "417", 418: "418", 428: "428", 429: "429",
		431: "431", 451: "451",
		500: "500", 501: "501", 502: "502", 503: "503", 504: "504", 505: "505", 511: "511",
	}
)

// responseCodeAttribute sets the response code agent attribute.
func responseCodeAttribute(a *attributes, code int) {
	rc := statusCodeLookup[code]
	if rc == "" {
		rc = strconv.Itoa(code)
	}
	a.Agent.Add(AttributeResponseCode, "", code)
	a.Agent.Add(AttributeResponseCodeDeprecated, rc, nil)
}
