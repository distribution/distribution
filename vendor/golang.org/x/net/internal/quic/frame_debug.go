// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.21

package quic

import (
	"fmt"
)

// A debugFrame is a representation of the contents of a QUIC frame,
// used for debug logs and testing but not the primary serving path.
type debugFrame interface {
	String() string
	write(w *packetWriter) bool
}

func parseDebugFrame(b []byte) (f debugFrame, n int) {
	if len(b) == 0 {
		return nil, -1
	}
	switch b[0] {
	case frameTypePadding:
		f, n = parseDebugFramePadding(b)
	case frameTypePing:
		f, n = parseDebugFramePing(b)
	case frameTypeAck, frameTypeAckECN:
		f, n = parseDebugFrameAck(b)
	case frameTypeResetStream:
		f, n = parseDebugFrameResetStream(b)
	case frameTypeStopSending:
		f, n = parseDebugFrameStopSending(b)
	case frameTypeCrypto:
		f, n = parseDebugFrameCrypto(b)
	case frameTypeNewToken:
		f, n = parseDebugFrameNewToken(b)
	case frameTypeStreamBase, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f:
		f, n = parseDebugFrameStream(b)
	case frameTypeMaxData:
		f, n = parseDebugFrameMaxData(b)
	case frameTypeMaxStreamData:
		f, n = parseDebugFrameMaxStreamData(b)
	case frameTypeMaxStreamsBidi, frameTypeMaxStreamsUni:
		f, n = parseDebugFrameMaxStreams(b)
	case frameTypeDataBlocked:
		f, n = parseDebugFrameDataBlocked(b)
	case frameTypeStreamDataBlocked:
		f, n = parseDebugFrameStreamDataBlocked(b)
	case frameTypeStreamsBlockedBidi, frameTypeStreamsBlockedUni:
		f, n = parseDebugFrameStreamsBlocked(b)
	case frameTypeNewConnectionID:
		f, n = parseDebugFrameNewConnectionID(b)
	case frameTypeRetireConnectionID:
		f, n = parseDebugFrameRetireConnectionID(b)
	case frameTypePathChallenge:
		f, n = parseDebugFramePathChallenge(b)
	case frameTypePathResponse:
		f, n = parseDebugFramePathResponse(b)
	case frameTypeConnectionCloseTransport:
		f, n = parseDebugFrameConnectionCloseTransport(b)
	case frameTypeConnectionCloseApplication:
		f, n = parseDebugFrameConnectionCloseApplication(b)
	case frameTypeHandshakeDone:
		f, n = parseDebugFrameHandshakeDone(b)
	default:
		return nil, -1
	}
	return f, n
}

// debugFramePadding is a sequence of PADDING frames.
type debugFramePadding struct {
	size int
}

func parseDebugFramePadding(b []byte) (f debugFramePadding, n int) {
	for n < len(b) && b[n] == frameTypePadding {
		n++
	}
	f.size = n
	return f, n
}

func (f debugFramePadding) String() string {
	return fmt.Sprintf("PADDING*%v", f.size)
}

func (f debugFramePadding) write(w *packetWriter) bool {
	if w.avail() == 0 {
		return false
	}
	for i := 0; i < f.size && w.avail() > 0; i++ {
		w.b = append(w.b, frameTypePadding)
	}
	return true
}

// debugFramePing is a PING frame.
type debugFramePing struct{}

func parseDebugFramePing(b []byte) (f debugFramePing, n int) {
	return f, 1
}

func (f debugFramePing) String() string {
	return "PING"
}

func (f debugFramePing) write(w *packetWriter) bool {
	return w.appendPingFrame()
}

// debugFrameAck is an ACK frame.
type debugFrameAck struct {
	ackDelay unscaledAckDelay
	ranges   []i64range[packetNumber]
}

func parseDebugFrameAck(b []byte) (f debugFrameAck, n int) {
	f.ranges = nil
	_, f.ackDelay, n = consumeAckFrame(b, func(_ int, start, end packetNumber) {
		f.ranges = append(f.ranges, i64range[packetNumber]{
			start: start,
			end:   end,
		})
	})
	// Ranges are parsed smallest to highest; reverse ranges slice to order them high to low.
	for i := 0; i < len(f.ranges)/2; i++ {
		j := len(f.ranges) - 1
		f.ranges[i], f.ranges[j] = f.ranges[j], f.ranges[i]
	}
	return f, n
}

func (f debugFrameAck) String() string {
	s := fmt.Sprintf("ACK Delay=%v", f.ackDelay)
	for _, r := range f.ranges {
		s += fmt.Sprintf(" [%v,%v)", r.start, r.end)
	}
	return s
}

func (f debugFrameAck) write(w *packetWriter) bool {
	return w.appendAckFrame(rangeset[packetNumber](f.ranges), f.ackDelay)
}

// debugFrameResetStream is a RESET_STREAM frame.
type debugFrameResetStream struct {
	id        streamID
	code      uint64
	finalSize int64
}

func parseDebugFrameResetStream(b []byte) (f debugFrameResetStream, n int) {
	f.id, f.code, f.finalSize, n = consumeResetStreamFrame(b)
	return f, n
}

func (f debugFrameResetStream) String() string {
	return fmt.Sprintf("RESET_STREAM ID=%v Code=%v FinalSize=%v", f.id, f.code, f.finalSize)
}

func (f debugFrameResetStream) write(w *packetWriter) bool {
	return w.appendResetStreamFrame(f.id, f.code, f.finalSize)
}

// debugFrameStopSending is a STOP_SENDING frame.
type debugFrameStopSending struct {
	id   streamID
	code uint64
}

func parseDebugFrameStopSending(b []byte) (f debugFrameStopSending, n int) {
	f.id, f.code, n = consumeStopSendingFrame(b)
	return f, n
}

func (f debugFrameStopSending) String() string {
	return fmt.Sprintf("STOP_SENDING ID=%v Code=%v", f.id, f.code)
}

func (f debugFrameStopSending) write(w *packetWriter) bool {
	return w.appendStopSendingFrame(f.id, f.code)
}

// debugFrameCrypto is a CRYPTO frame.
type debugFrameCrypto struct {
	off  int64
	data []byte
}

func parseDebugFrameCrypto(b []byte) (f debugFrameCrypto, n int) {
	f.off, f.data, n = consumeCryptoFrame(b)
	return f, n
}

func (f debugFrameCrypto) String() string {
	return fmt.Sprintf("CRYPTO Offset=%v Length=%v", f.off, len(f.data))
}

func (f debugFrameCrypto) write(w *packetWriter) bool {
	b, added := w.appendCryptoFrame(f.off, len(f.data))
	copy(b, f.data)
	return added
}

// debugFrameNewToken is a NEW_TOKEN frame.
type debugFrameNewToken struct {
	token []byte
}

func parseDebugFrameNewToken(b []byte) (f debugFrameNewToken, n int) {
	f.token, n = consumeNewTokenFrame(b)
	return f, n
}

func (f debugFrameNewToken) String() string {
	return fmt.Sprintf("NEW_TOKEN Token=%x", f.token)
}

func (f debugFrameNewToken) write(w *packetWriter) bool {
	return w.appendNewTokenFrame(f.token)
}

// debugFrameStream is a STREAM frame.
type debugFrameStream struct {
	id   streamID
	fin  bool
	off  int64
	data []byte
}

func parseDebugFrameStream(b []byte) (f debugFrameStream, n int) {
	f.id, f.off, f.fin, f.data, n = consumeStreamFrame(b)
	return f, n
}

func (f debugFrameStream) String() string {
	fin := ""
	if f.fin {
		fin = " FIN"
	}
	return fmt.Sprintf("STREAM ID=%v%v Offset=%v Length=%v", f.id, fin, f.off, len(f.data))
}

func (f debugFrameStream) write(w *packetWriter) bool {
	b, added := w.appendStreamFrame(f.id, f.off, len(f.data), f.fin)
	copy(b, f.data)
	return added
}

// debugFrameMaxData is a MAX_DATA frame.
type debugFrameMaxData struct {
	max int64
}

func parseDebugFrameMaxData(b []byte) (f debugFrameMaxData, n int) {
	f.max, n = consumeMaxDataFrame(b)
	return f, n
}

func (f debugFrameMaxData) String() string {
	return fmt.Sprintf("MAX_DATA Max=%v", f.max)
}

func (f debugFrameMaxData) write(w *packetWriter) bool {
	return w.appendMaxDataFrame(f.max)
}

// debugFrameMaxStreamData is a MAX_STREAM_DATA frame.
type debugFrameMaxStreamData struct {
	id  streamID
	max int64
}

func parseDebugFrameMaxStreamData(b []byte) (f debugFrameMaxStreamData, n int) {
	f.id, f.max, n = consumeMaxStreamDataFrame(b)
	return f, n
}

func (f debugFrameMaxStreamData) String() string {
	return fmt.Sprintf("MAX_STREAM_DATA ID=%v Max=%v", f.id, f.max)
}

func (f debugFrameMaxStreamData) write(w *packetWriter) bool {
	return w.appendMaxStreamDataFrame(f.id, f.max)
}

// debugFrameMaxStreams is a MAX_STREAMS frame.
type debugFrameMaxStreams struct {
	streamType streamType
	max        int64
}

func parseDebugFrameMaxStreams(b []byte) (f debugFrameMaxStreams, n int) {
	f.streamType, f.max, n = consumeMaxStreamsFrame(b)
	return f, n
}

func (f debugFrameMaxStreams) String() string {
	return fmt.Sprintf("MAX_STREAMS Type=%v Max=%v", f.streamType, f.max)
}

func (f debugFrameMaxStreams) write(w *packetWriter) bool {
	return w.appendMaxStreamsFrame(f.streamType, f.max)
}

// debugFrameDataBlocked is a DATA_BLOCKED frame.
type debugFrameDataBlocked struct {
	max int64
}

func parseDebugFrameDataBlocked(b []byte) (f debugFrameDataBlocked, n int) {
	f.max, n = consumeDataBlockedFrame(b)
	return f, n
}

func (f debugFrameDataBlocked) String() string {
	return fmt.Sprintf("DATA_BLOCKED Max=%v", f.max)
}

func (f debugFrameDataBlocked) write(w *packetWriter) bool {
	return w.appendDataBlockedFrame(f.max)
}

// debugFrameStreamDataBlocked is a STREAM_DATA_BLOCKED frame.
type debugFrameStreamDataBlocked struct {
	id  streamID
	max int64
}

func parseDebugFrameStreamDataBlocked(b []byte) (f debugFrameStreamDataBlocked, n int) {
	f.id, f.max, n = consumeStreamDataBlockedFrame(b)
	return f, n
}

func (f debugFrameStreamDataBlocked) String() string {
	return fmt.Sprintf("STREAM_DATA_BLOCKED ID=%v Max=%v", f.id, f.max)
}

func (f debugFrameStreamDataBlocked) write(w *packetWriter) bool {
	return w.appendStreamDataBlockedFrame(f.id, f.max)
}

// debugFrameStreamsBlocked is a STREAMS_BLOCKED frame.
type debugFrameStreamsBlocked struct {
	streamType streamType
	max        int64
}

func parseDebugFrameStreamsBlocked(b []byte) (f debugFrameStreamsBlocked, n int) {
	f.streamType, f.max, n = consumeStreamsBlockedFrame(b)
	return f, n
}

func (f debugFrameStreamsBlocked) String() string {
	return fmt.Sprintf("STREAMS_BLOCKED Type=%v Max=%v", f.streamType, f.max)
}

func (f debugFrameStreamsBlocked) write(w *packetWriter) bool {
	return w.appendStreamsBlockedFrame(f.streamType, f.max)
}

// debugFrameNewConnectionID is a NEW_CONNECTION_ID frame.
type debugFrameNewConnectionID struct {
	seq           int64
	retirePriorTo int64
	connID        []byte
	token         statelessResetToken
}

func parseDebugFrameNewConnectionID(b []byte) (f debugFrameNewConnectionID, n int) {
	f.seq, f.retirePriorTo, f.connID, f.token, n = consumeNewConnectionIDFrame(b)
	return f, n
}

func (f debugFrameNewConnectionID) String() string {
	return fmt.Sprintf("NEW_CONNECTION_ID Seq=%v Retire=%v ID=%x Token=%x", f.seq, f.retirePriorTo, f.connID, f.token[:])
}

func (f debugFrameNewConnectionID) write(w *packetWriter) bool {
	return w.appendNewConnectionIDFrame(f.seq, f.retirePriorTo, f.connID, f.token)
}

// debugFrameRetireConnectionID is a NEW_CONNECTION_ID frame.
type debugFrameRetireConnectionID struct {
	seq int64
}

func parseDebugFrameRetireConnectionID(b []byte) (f debugFrameRetireConnectionID, n int) {
	f.seq, n = consumeRetireConnectionIDFrame(b)
	return f, n
}

func (f debugFrameRetireConnectionID) String() string {
	return fmt.Sprintf("RETIRE_CONNECTION_ID Seq=%v", f.seq)
}

func (f debugFrameRetireConnectionID) write(w *packetWriter) bool {
	return w.appendRetireConnectionIDFrame(f.seq)
}

// debugFramePathChallenge is a PATH_CHALLENGE frame.
type debugFramePathChallenge struct {
	data uint64
}

func parseDebugFramePathChallenge(b []byte) (f debugFramePathChallenge, n int) {
	f.data, n = consumePathChallengeFrame(b)
	return f, n
}

func (f debugFramePathChallenge) String() string {
	return fmt.Sprintf("PATH_CHALLENGE Data=%016x", f.data)
}

func (f debugFramePathChallenge) write(w *packetWriter) bool {
	return w.appendPathChallengeFrame(f.data)
}

// debugFramePathResponse is a PATH_RESPONSE frame.
type debugFramePathResponse struct {
	data uint64
}

func parseDebugFramePathResponse(b []byte) (f debugFramePathResponse, n int) {
	f.data, n = consumePathResponseFrame(b)
	return f, n
}

func (f debugFramePathResponse) String() string {
	return fmt.Sprintf("PATH_RESPONSE Data=%016x", f.data)
}

func (f debugFramePathResponse) write(w *packetWriter) bool {
	return w.appendPathResponseFrame(f.data)
}

// debugFrameConnectionCloseTransport is a CONNECTION_CLOSE frame carrying a transport error.
type debugFrameConnectionCloseTransport struct {
	code      transportError
	frameType uint64
	reason    string
}

func parseDebugFrameConnectionCloseTransport(b []byte) (f debugFrameConnectionCloseTransport, n int) {
	f.code, f.frameType, f.reason, n = consumeConnectionCloseTransportFrame(b)
	return f, n
}

func (f debugFrameConnectionCloseTransport) String() string {
	s := fmt.Sprintf("CONNECTION_CLOSE Code=%v", f.code)
	if f.frameType != 0 {
		s += fmt.Sprintf(" FrameType=%v", f.frameType)
	}
	if f.reason != "" {
		s += fmt.Sprintf(" Reason=%q", f.reason)
	}
	return s
}

func (f debugFrameConnectionCloseTransport) write(w *packetWriter) bool {
	return w.appendConnectionCloseTransportFrame(f.code, f.frameType, f.reason)
}

// debugFrameConnectionCloseApplication is a CONNECTION_CLOSE frame carrying an application error.
type debugFrameConnectionCloseApplication struct {
	code   uint64
	reason string
}

func parseDebugFrameConnectionCloseApplication(b []byte) (f debugFrameConnectionCloseApplication, n int) {
	f.code, f.reason, n = consumeConnectionCloseApplicationFrame(b)
	return f, n
}

func (f debugFrameConnectionCloseApplication) String() string {
	s := fmt.Sprintf("CONNECTION_CLOSE AppCode=%v", f.code)
	if f.reason != "" {
		s += fmt.Sprintf(" Reason=%q", f.reason)
	}
	return s
}

func (f debugFrameConnectionCloseApplication) write(w *packetWriter) bool {
	return w.appendConnectionCloseApplicationFrame(f.code, f.reason)
}

// debugFrameHandshakeDone is a HANDSHAKE_DONE frame.
type debugFrameHandshakeDone struct{}

func parseDebugFrameHandshakeDone(b []byte) (f debugFrameHandshakeDone, n int) {
	return f, 1
}

func (f debugFrameHandshakeDone) String() string {
	return "HANDSHAKE_DONE"
}

func (f debugFrameHandshakeDone) write(w *packetWriter) bool {
	return w.appendHandshakeDoneFrame()
}
