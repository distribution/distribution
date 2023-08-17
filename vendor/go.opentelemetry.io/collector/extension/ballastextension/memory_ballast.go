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
	"context"

	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
)

const megaBytes = 1024 * 1024

type MemoryBallast struct {
	cfg              *Config
	logger           *zap.Logger
	ballast          []byte
	ballastSizeBytes uint64
	getTotalMem      func() (uint64, error)
}

func (m *MemoryBallast) Start(_ context.Context, _ component.Host) error {
	// absolute value supersedes percentage setting
	if m.cfg.SizeMiB > 0 {
		m.ballastSizeBytes = m.cfg.SizeMiB * megaBytes
	} else {
		totalMemory, err := m.getTotalMem()
		if err != nil {
			return err
		}
		ballastPercentage := m.cfg.SizeInPercentage
		m.ballastSizeBytes = ballastPercentage * totalMemory / 100
	}

	if m.ballastSizeBytes > 0 {
		m.ballast = make([]byte, m.ballastSizeBytes)
	}

	m.logger.Info("Setting memory ballast", zap.Uint32("MiBs", uint32(m.ballastSizeBytes/megaBytes)))

	return nil
}

func (m *MemoryBallast) Shutdown(_ context.Context) error {
	m.ballast = nil
	return nil
}

func newMemoryBallast(cfg *Config, logger *zap.Logger, getTotalMem func() (uint64, error)) *MemoryBallast {
	return &MemoryBallast{
		cfg:         cfg,
		logger:      logger,
		getTotalMem: getTotalMem,
	}
}

// GetBallastSize returns the current ballast memory setting in bytes
func (m *MemoryBallast) GetBallastSize() uint64 {
	return m.ballastSizeBytes
}
