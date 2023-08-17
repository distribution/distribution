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

package configtelemetry // import "go.opentelemetry.io/collector/config/configtelemetry"

import (
	"encoding"
	"errors"
	"fmt"
	"strings"
)

const (
	// LevelNone indicates that no telemetry data should be collected.
	LevelNone Level = iota - 1
	// LevelBasic is the recommended and covers the basics of the service telemetry.
	LevelBasic
	// LevelNormal adds some other indicators on top of basic.
	LevelNormal
	// LevelDetailed adds dimensions and views to the previous levels.
	LevelDetailed

	levelNoneStr     = "none"
	levelBasicStr    = "basic"
	levelNormalStr   = "normal"
	levelDetailedStr = "detailed"
)

// Level is the level of internal telemetry (metrics, logs, traces about the component itself)
// that every component should generate.
type Level int32

var _ encoding.TextUnmarshaler = (*Level)(nil)

func (l Level) String() string {
	switch l {
	case LevelNone:
		return levelNoneStr
	case LevelBasic:
		return levelBasicStr
	case LevelNormal:
		return levelNormalStr
	case LevelDetailed:
		return levelDetailedStr
	}
	return "unknown"
}

// UnmarshalText unmarshalls text to a Level.
func (l *Level) UnmarshalText(text []byte) error {
	if l == nil {
		return errors.New("cannot unmarshal to a nil *Level")
	}

	str := strings.ToLower(string(text))
	switch str {
	case levelNoneStr:
		*l = LevelNone
		return nil
	case levelBasicStr:
		*l = LevelBasic
		return nil
	case levelNormalStr:
		*l = LevelNormal
		return nil
	case levelDetailedStr:
		*l = LevelDetailed
		return nil
	}
	return fmt.Errorf("unknown metrics level %q", str)
}
