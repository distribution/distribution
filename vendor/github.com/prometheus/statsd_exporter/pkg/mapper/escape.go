// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"strings"
	"unicode/utf8"
)

// EscapeMetricName replaces invalid characters in the metric name with "_"
// Valid characters are a-z, A-Z, 0-9, and _
func EscapeMetricName(metricName string) string {
	metricLen := len(metricName)
	if metricLen == 0 {
		return ""
	}

	escaped := false
	var sb strings.Builder
	// If a metric starts with a digit, allocate the memory and prepend an
	// underscore.
	if metricName[0] >= '0' && metricName[0] <= '9' {
		escaped = true
		sb.Grow(metricLen + 1)
		sb.WriteByte('_')
	}

	// This is an character replacement method optimized for this limited
	// use case.  It is much faster than using a regex.
	offset := 0
	for i, c := range metricName {
		// Seek forward, skipping valid characters until we find one that needs
		// to be replaced, then add all the characters we've seen so far to the
		// string.Builder.
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || (c == '_') {
			// Character is valid, so skip over it without doing anything.
		} else {
			if !escaped {
				// Up until now we've been lazy and avoided actually allocating
				// memory.  Unfortunately we've now determined this string needs
				// escaping, so allocate the buffer for the whole string.
				escaped = true
				sb.Grow(metricLen)
			}
			sb.WriteString(metricName[offset:i])
			offset = i + utf8.RuneLen(c)
			sb.WriteByte('_')
		}
	}

	if !escaped {
		// This is the happy path where nothing had to be escaped, so we can
		// avoid doing anything.
		return metricName
	}

	if offset < metricLen {
		sb.WriteString(metricName[offset:])
	}

	return sb.String()
}
