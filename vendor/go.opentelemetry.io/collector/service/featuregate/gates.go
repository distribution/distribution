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
	"fmt"
	"sync"
)

// Gate represents an individual feature that may be enabled or disabled based
// on the lifecycle state of the feature and CLI flags specified by the user.
type Gate struct {
	ID          string
	Description string
	Enabled     bool
}

var reg = NewRegistry()

// GetRegistry returns the global Registry.
func GetRegistry() *Registry {
	return reg
}

// NewRegistry returns a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{gates: make(map[string]Gate)}
}

type Registry struct {
	mu    sync.RWMutex
	gates map[string]Gate
}

// Apply a configuration in the form of a map of Gate identifiers to boolean values.
// Sets only those values provided in the map, other gate values are not changed.
func (r *Registry) Apply(cfg map[string]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, val := range cfg {
		g, ok := r.gates[id]
		if !ok {
			return fmt.Errorf("feature gate %s is unregistered", id)
		}
		g.Enabled = val
		r.gates[g.ID] = g
	}
	return nil
}

// Deprecated: [v0.58.0] Use Apply instead.
func (r *Registry) MustApply(cfg map[string]bool) {
	if err := r.Apply(cfg); err != nil {
		panic(err)
	}
}

// IsEnabled returns true if a registered feature gate is enabled and false otherwise.
func (r *Registry) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.gates[id]
	return ok && g.Enabled
}

// MustRegister like Register but panics if a Gate with the same ID is already registered.
func (r *Registry) MustRegister(g Gate) {
	if err := r.Register(g); err != nil {
		panic(err)
	}
}

// Register registers a Gate. May only be called in an init() function.
func (r *Registry) Register(g Gate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.gates[g.ID]; ok {
		return fmt.Errorf("attempted to add pre-existing gate %q", g.ID)
	}
	r.gates[g.ID] = g
	return nil
}

// List returns a slice of copies of all registered Gates.
func (r *Registry) List() []Gate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]Gate, len(r.gates))
	i := 0
	for _, gate := range r.gates {
		ret[i] = gate
		i++
	}

	return ret
}
