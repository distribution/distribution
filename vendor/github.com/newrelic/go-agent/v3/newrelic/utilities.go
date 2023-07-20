// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// jsonString assists in logging JSON:  Based on the formatter used to log
// Context contents, the contents could be marshalled as JSON or just printed
// directly.
type jsonString string

// MarshalJSON returns the jsonString unmodified without any escaping.
func (js jsonString) MarshalJSON() ([]byte, error) {
	if "" == js {
		return []byte("null"), nil
	}
	return []byte(js), nil
}

func removeFirstSegment(name string) string {
	idx := strings.Index(name, "/")
	if -1 == idx {
		return name
	}
	return name[idx+1:]
}

func timeToIntMillis(t time.Time) int64 {
	return t.UnixNano() / (1000 * 1000)
}

func timeToFloatMilliseconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(1000*1000)
}

// compactJSONString removes the whitespace from a JSON string.  This function
// will panic if the string provided is not valid JSON.  Thus is must only be
// used in testing code!
func compactJSONString(js string) string {
	buf := new(bytes.Buffer)
	if err := json.Compact(buf, []byte(js)); err != nil {
		panic(fmt.Errorf("unable to compact JSON: %v", err))
	}
	return buf.String()
}

// getContentLengthFromHeader gets the content length from a HTTP header, or -1
// if no content length is available.
func getContentLengthFromHeader(h http.Header) int64 {
	if cl := h.Get("Content-Length"); cl != "" {
		if contentLength, err := strconv.ParseInt(cl, 10, 64); err == nil {
			return contentLength
		}
	}

	return -1
}

// stringLengthByteLimit truncates strings using a byte-limit boundary and
// avoids terminating in the middle of a multibyte character.
func stringLengthByteLimit(str string, byteLimit int) string {
	if len(str) <= byteLimit {
		return str
	}

	limitIndex := 0
	for pos := range str {
		if pos > byteLimit {
			break
		}
		limitIndex = pos
	}
	return str[0:limitIndex]
}

func timeFromUnixMilliseconds(millis uint64) time.Time {
	secs := int64(millis) / 1000
	msecsRemaining := int64(millis) % 1000
	nsecsRemaining := msecsRemaining * (1000 * 1000)
	return time.Unix(secs, nsecsRemaining)
}

// timeToUnixMilliseconds converts a time into a Unix timestamp in millisecond
// units.
func timeToUnixMilliseconds(tm time.Time) uint64 {
	return uint64(tm.UnixNano()) / uint64(1000*1000)
}

// minorVersion takes a given version string and returns only the major and
// minor portions of it. If the input is malformed, it returns the input
// untouched.
func minorVersion(v string) string {
	split := strings.SplitN(v, ".", 3)
	if len(split) < 2 {
		return v
	}
	return split[0] + "." + split[1]
}
