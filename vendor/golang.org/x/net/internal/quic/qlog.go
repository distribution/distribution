// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.21

package quic

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/netip"
)

// Log levels for qlog events.
const (
	// QLogLevelFrame includes per-frame information.
	// When this level is enabled, packet_sent and packet_received events will
	// contain information on individual frames sent/received.
	QLogLevelFrame = slog.Level(-6)

	// QLogLevelPacket events occur at most once per packet sent or received.
	//
	// For example: packet_sent, packet_received.
	QLogLevelPacket = slog.Level(-4)

	// QLogLevelConn events occur multiple times over a connection's lifetime,
	// but less often than the frequency of individual packets.
	//
	// For example: connection_state_updated.
	QLogLevelConn = slog.Level(-2)

	// QLogLevelEndpoint events occur at most once per connection.
	//
	// For example: connection_started, connection_closed.
	QLogLevelEndpoint = slog.Level(0)
)

func (c *Conn) logEnabled(level slog.Level) bool {
	return c.log != nil && c.log.Enabled(context.Background(), level)
}

// slogHexstring returns a slog.Attr for a value of the hexstring type.
//
// https://www.ietf.org/archive/id/draft-ietf-quic-qlog-main-schema-04.html#section-1.1.1
func slogHexstring(key string, value []byte) slog.Attr {
	return slog.String(key, hex.EncodeToString(value))
}

func slogAddr(key string, value netip.Addr) slog.Attr {
	return slog.String(key, value.String())
}

func (c *Conn) logConnectionStarted(originalDstConnID []byte, peerAddr netip.AddrPort) {
	if c.config.QLogLogger == nil ||
		!c.config.QLogLogger.Enabled(context.Background(), QLogLevelEndpoint) {
		return
	}
	var vantage string
	if c.side == clientSide {
		vantage = "client"
		originalDstConnID = c.connIDState.originalDstConnID
	} else {
		vantage = "server"
	}
	// A qlog Trace container includes some metadata (title, description, vantage_point)
	// and a list of Events. The Trace also includes a common_fields field setting field
	// values common to all events in the trace.
	//
	//	Trace = {
	//	    ? title: text
	//	    ? description: text
	//	    ? configuration: Configuration
	//	    ? common_fields: CommonFields
	//	    ? vantage_point: VantagePoint
	//	    events: [* Event]
	//	}
	//
	// To map this into slog's data model, we start each per-connection trace with a With
	// call that includes both the trace metadata and the common fields.
	//
	// This means that in slog's model, each trace event will also include
	// the Trace metadata fields (vantage_point), which is a divergence from the qlog model.
	c.log = c.config.QLogLogger.With(
		// The group_id permits associating traces taken from different vantage points
		// for the same connection.
		//
		// We use the original destination connection ID as the group ID.
		//
		// https://www.ietf.org/archive/id/draft-ietf-quic-qlog-main-schema-04.html#section-3.4.6
		slogHexstring("group_id", originalDstConnID),
		slog.Group("vantage_point",
			slog.String("name", "go quic"),
			slog.String("type", vantage),
		),
	)
	localAddr := c.endpoint.LocalAddr()
	// https://www.ietf.org/archive/id/draft-ietf-quic-qlog-quic-events-03.html#section-4.2
	c.log.LogAttrs(context.Background(), QLogLevelEndpoint,
		"connectivity:connection_started",
		slogAddr("src_ip", localAddr.Addr()),
		slog.Int("src_port", int(localAddr.Port())),
		slogHexstring("src_cid", c.connIDState.local[0].cid),
		slogAddr("dst_ip", peerAddr.Addr()),
		slog.Int("dst_port", int(peerAddr.Port())),
		slogHexstring("dst_cid", c.connIDState.remote[0].cid),
	)
}

func (c *Conn) logConnectionClosed() {
	if !c.logEnabled(QLogLevelEndpoint) {
		return
	}
	err := c.lifetime.finalErr
	trigger := "error"
	switch e := err.(type) {
	case *ApplicationError:
		// TODO: Distinguish between peer and locally-initiated close.
		trigger = "application"
	case localTransportError:
		switch err {
		case errHandshakeTimeout:
			trigger = "handshake_timeout"
		default:
			if e.code == errNo {
				trigger = "clean"
			}
		}
	case peerTransportError:
		if e.code == errNo {
			trigger = "clean"
		}
	default:
		switch err {
		case errIdleTimeout:
			trigger = "idle_timeout"
		case errStatelessReset:
			trigger = "stateless_reset"
		}
	}
	// https://www.ietf.org/archive/id/draft-ietf-quic-qlog-quic-events-03.html#section-4.3
	c.log.LogAttrs(context.Background(), QLogLevelEndpoint,
		"connectivity:connection_closed",
		slog.String("trigger", trigger),
	)
}
