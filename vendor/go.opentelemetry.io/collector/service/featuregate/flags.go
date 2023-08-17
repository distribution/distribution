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

package featuregate // import "go.opentelemetry.io/collector/service/featuregate"

import (
	"flag"
	"sort"
	"strings"
)

var _ flag.Value = (*FlagValue)(nil)

// FlagValue implements the flag.Value interface and provides a mechanism for applying feature
// gate statuses to a Registry
type FlagValue map[string]bool

// String returns a string representing the FlagValue
func (f FlagValue) String() string {
	var t []string
	for k, v := range f {
		if v {
			t = append(t, k)
		} else {
			t = append(t, "-"+k)
		}
	}

	// Sort the list of identifiers for consistent results
	sort.Strings(t)
	return strings.Join(t, ",")
}

// Set applies the FlagValue encoded in the input string
func (f FlagValue) Set(s string) error {
	if s == "" {
		return nil
	}

	return f.setSlice(strings.Split(s, ","))
}

func (f FlagValue) setSlice(s []string) error {
	for _, v := range s {
		var id string
		var val bool
		switch v[0] {
		case '-':
			id = v[1:]
			val = false
		case '+':
			id = v[1:]
			val = true
		default:
			id = v
			val = true
		}

		if _, exists := f[id]; exists {
			// If the status has already been set, ignore it
			// This allows CLI flags, which are processed first
			// to take precedence over config settings
			continue
		}
		f[id] = val
	}

	return nil
}
