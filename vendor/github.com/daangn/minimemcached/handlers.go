package minimemcached

import (
	"net"
	"strconv"
	"strings"
)

// handleGet() handles `get` request.
func handleGet(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) == 1 {
		_, _ = conn.Write(resultErr)
		return
	}
	key := cmdLine[2]

	m.gets([]string{key}, conn)
}

// handleGets() handles `gets` request.
func handleGets(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) == 1 {
		_, _ = conn.Write(resultErr)
		return
	}
	cmdLine[len(cmdLine)-1] = strings.TrimSuffix(cmdLine[len(cmdLine)-1], string(crlf))

	m.gets(cmdLine[1:], conn)
}

// handleSet() handles `set` request.
func handleSet(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 5 {
		_, _ = conn.Write(resultErr)
		return
	}
	key := cmdLine[1]

	flags, err := strconv.ParseUint(cmdLine[2], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	expiration, err := strconv.ParseInt(cmdLine[3], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	item := &item{
		flags:      uint32(flags),
		value:      value,
		expiration: int32(expiration),
		createdAt:  m.clock.Now().Unix(),
	}

	m.set(key, item, bytes, conn)
}

// handleAdd() handles `add` request.
func handleAdd(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 5 {
		_, _ = conn.Write(resultErr)
		return
	}
	key := cmdLine[1]

	flags, err := strconv.ParseUint(cmdLine[2], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	expiration, err := strconv.ParseInt(cmdLine[3], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	item := &item{
		flags:      uint32(flags),
		value:      value,
		expiration: int32(expiration),
		createdAt:  m.clock.Now().Unix(),
	}

	m.add(key, item, bytes, conn)
}

// handleReplace() handles `replace` request.
func handleReplace(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 5 {
		_, _ = conn.Write(resultErr)
		return
	}
	key := cmdLine[1]

	flags, err := strconv.ParseUint(cmdLine[2], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	expiration, err := strconv.ParseInt(cmdLine[3], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	item := &item{
		flags:      uint32(flags),
		value:      value,
		expiration: int32(expiration),
		createdAt:  m.clock.Now().Unix(),
	}

	m.replace(key, item, bytes, conn)
}

// handleAppend() handles `append` requests.
func handleAppend(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 5 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]
	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	m.append(key, bytes, value, conn)
}

// handlePrepend() handles `prepend` requests.
func handlePrepend(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 5 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]

	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	m.prepend(key, bytes, value, conn)
}

// handleDelete() handles `delete` requests.
func handleDelete(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) != 2 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]

	m.delete(key, conn)
}

// handleIncr() handles `incr` requests.
func handleIncr(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) != 3 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]
	incrValue := cmdLine[2]
	numericIncrValue, isNumeric := getNumericValueFromString(incrValue)
	if !isNumeric {
		_, _ = conn.Write(resultClientErrInvalidNumericDeltaArg)
		return
	}

	m.incr(key, numericIncrValue, conn)
}

// handleDecr() handles `decr` requests.
func handleDecr(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) != 3 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]
	decrValue := cmdLine[2]
	numericDecrValue, isNumeric := getNumericValueFromString(decrValue)
	if !isNumeric {
		_, _ = conn.Write(resultClientErrInvalidNumericDeltaArg)
		return
	}

	m.decr(key, numericDecrValue, conn)
}

// handleTouch() handles `touch` requests.
func handleTouch(m *MiniMemcached, cmdLine []string, conn net.Conn) {
	if len(cmdLine) != 3 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]
	expTime := cmdLine[2]

	expiration, err := strconv.ParseInt(expTime, 10, 32)
	if err != nil {
		_, _ = conn.Write(resultClientErrInvalidExpTimeArg)
		return
	}

	m.touch(key, int32(expiration), conn)
}

// handleCas() handles `cas` requests.
func handleCas(m *MiniMemcached, cmdLine []string, value []byte, conn net.Conn) {
	if len(cmdLine) != 6 {
		_, _ = conn.Write(resultErr)
		return
	}

	key := cmdLine[1]

	flags, err := strconv.ParseUint(cmdLine[2], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	expiration, err := strconv.ParseInt(cmdLine[3], 0, 32)
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	bytes, err := strconv.Atoi(cmdLine[4])
	if err != nil {
		_, _ = conn.Write(resultErr)
		return
	}

	casToken, isNumeric := getNumericValueFromString(cmdLine[5])
	if !isNumeric {
		_, _ = conn.Write(resultClientErrBadCliFormat)
		return
	}

	item := &item{
		flags:      uint32(flags),
		value:      value,
		expiration: int32(expiration),
		createdAt:  m.clock.Now().Unix(),
	}

	m.cas(key, item, bytes, casToken, conn)
}

// handleFlushAll() handles memcached `flush_all` requests.
func handleFlushAll(m *MiniMemcached, conn net.Conn) {
	m.flushAll(conn)
}

// handleVersion() handles memcached `version` requests.
func handleVersion(m *MiniMemcached, conn net.Conn) {
	m.version(conn)
}

// handleErr() returns error to client when invalid request is made.
func handleErr(conn net.Conn) {
	_, _ = conn.Write(resultErr)
}
