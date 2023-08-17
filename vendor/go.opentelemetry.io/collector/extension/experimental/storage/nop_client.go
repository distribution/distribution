// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage // import "go.opentelemetry.io/collector/extension/experimental/storage"

import "context"

type nopClient struct{}

var nopClientInstance Client = &nopClient{}

// NewNopClient returns a nop client
func NewNopClient() Client {
	return nopClientInstance
}

// Get does nothing, and returns nil, nil
func (c nopClient) Get(context.Context, string) ([]byte, error) {
	return nil, nil // no result, but no problem
}

// Set does nothing and returns nil
func (c nopClient) Set(context.Context, string, []byte) error {
	return nil // no problem
}

// Delete does nothing and returns nil
func (c nopClient) Delete(context.Context, string) error {
	return nil // no problem
}

// Close does nothing and returns nil
func (c nopClient) Close(context.Context) error {
	return nil
}

// Batch does nothing, and returns nil, nil
func (c nopClient) Batch(context.Context, ...Operation) error {
	return nil // no result, but no problem
}
