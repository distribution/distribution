// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.21

package quic

import (
	"net/netip"
	"sync"
)

type datagram struct {
	b    []byte
	addr netip.AddrPort
}

var datagramPool = sync.Pool{
	New: func() any {
		return &datagram{
			b: make([]byte, maxUDPPayloadSize),
		}
	},
}

func newDatagram() *datagram {
	m := datagramPool.Get().(*datagram)
	m.b = m.b[:cap(m.b)]
	return m
}

func (m *datagram) recycle() {
	if cap(m.b) != maxUDPPayloadSize {
		return
	}
	datagramPool.Put(m)
}
