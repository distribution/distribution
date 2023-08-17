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

package ballastextension // import "go.opentelemetry.io/collector/extension/ballastextension"

import (
	"errors"

	"go.opentelemetry.io/collector/config"
)

// Config has the configuration for the ballast extension.
type Config struct {
	config.ExtensionSettings `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct

	// SizeMiB is the size, in MiB, of the memory ballast
	// to be created for this process.
	SizeMiB uint64 `mapstructure:"size_mib"`

	// SizeInPercentage is the maximum amount of memory ballast, in %, targeted to be
	// allocated. The fixed memory settings SizeMiB has a higher precedence.
	SizeInPercentage uint64 `mapstructure:"size_in_percentage"`
}

// Validate checks if the extension configuration is valid
func (cfg *Config) Validate() error {
	// no need to validate less than 0 case for uint64
	if cfg.SizeInPercentage > 100 {
		return errors.New("size_in_percentage is not in range 0 to 100")
	}
	return nil
}
