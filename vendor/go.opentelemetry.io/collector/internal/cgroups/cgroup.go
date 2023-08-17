// Copyright  The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Keep the original Uber license.

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//go:build linux
// +build linux

package cgroups // import "go.opentelemetry.io/collector/internal/cgroups"

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// CGroup represents the data structure for a Linux control group.
type CGroup struct {
	path string
}

// NewCGroup returns a new *CGroup from a given path.
func NewCGroup(path string) *CGroup {
	return &CGroup{path: path}
}

// Path returns the path of the CGroup*.
func (cg *CGroup) Path() string {
	return cg.path
}

// ParamPath returns the path of the given cgroup param under itself.
func (cg *CGroup) ParamPath(param string) string {
	return filepath.Join(cg.path, param)
}

// readFirstLine reads the first line from a cgroup param file.
func (cg *CGroup) readFirstLine(param string) (string, error) {
	paramFile, err := os.Open(cg.ParamPath(param))
	if err != nil {
		return "", err
	}
	defer paramFile.Close()

	scanner := bufio.NewScanner(paramFile)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.ErrUnexpectedEOF
}

// readInt parses the first line from a cgroup param file as int.
func (cg *CGroup) readInt(param string) (int, error) {
	text, err := cg.readFirstLine(param)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(text)
}
