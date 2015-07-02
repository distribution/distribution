// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package icmp

// An Extension represents an ICMP extension.
type Extension interface {
	// Len returns the length of ICMP extension.
	Len() int

	// Marshal returns the binary enconding of ICMP extension.
	Marshal() ([]byte, error)
}

const extensionVersion = 2
