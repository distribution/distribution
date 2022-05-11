// Package multicodec exposes the multicodec table as Go constants.
package multicodec

import (
	"flag"
	"fmt"
	"strconv"
)

//go:generate go run gen.go
//go:generate gofmt -w code_table.go
//go:generate go run golang.org/x/tools/cmd/stringer@v0.1.7 -type=Code -linecomment

// Code describes an integer reserved in the multicodec table, defined at
// github.com/multiformats/multicodec.
type Code uint64

// Assert that Code implements flag.Value.
// Requires a pointer, since Set modifies the receiver.
//
// Note that we don't implement encoding.TextMarshaler and encoding.TextUnmarshaler.
// That's on purpose; even though multicodec names are stable just like the codes,
// Go should still generally encode and decode multicodecs by their code number.
// Many encoding libraries like xml and json default to TextMarshaler if it exists.
//
// Conversely, implementing flag.Value makes sense;
// --someflag=sha1 is useful as it would often be typed by a human.
var _ flag.Value = (*Code)(nil)

// Assert that Code implements fmt.Stringer without a pointer.
var _ fmt.Stringer = Code(0)

// ReservedStart is the (inclusive) start of the reserved range of codes that
// are safe to use for internal purposes.
const ReservedStart = 0x300000

// ReservedEnd is the (inclusive) end of the reserved range of codes that are
// safe to use for internal purposes.
const ReservedEnd = 0x3FFFFF

// Set implements flag.Value, interpreting the input string as a multicodec and
// setting the receiver to it.
//
// The input string can be the name or number for a known code. A number can be
// in any format accepted by strconv.ParseUint with base 0, including decimal
// and hexadecimal.
//
// Numbers in the reserved range 0x300000-0x3FFFFF are also accepted.
func (c *Code) Set(text string) error {
	// Checking if the text is a valid number is cheap, so do it first.
	// It should be impossible for a string to be both a valid number and a
	// valid name, anyway.
	if n, err := strconv.ParseUint(text, 0, 64); err == nil {
		code := Code(n)
		if code >= 0x300000 && code <= 0x3FFFFF { // reserved range
			*c = code
			return nil
		}
		if _, ok := _Code_map[code]; ok { // known code
			*c = code
			return nil
		}
	}

	// For now, checking if the text is a valid name is a linear operation,
	// so do it after.
	// Right now we have ~450 codes, so a linear search isn't too bad.
	// Consider generating a map[string]Code later on if linear search
	// starts being a problem.
	for code, name := range _Code_map {
		if name == text {
			*c = code
			return nil
		}
	}
	return fmt.Errorf("unknown multicodec: %q", text)
}

// Note that KnownCodes is a function backed by a code-generated slice.
// Later on, if the slice gets too large, we could codegen a packed form
// and only expand to a regular slice via a sync.Once.
// A function also makes it a bit clearer that the list should be read-only.

// KnownCodes returns a list of all codes registered in the multicodec table.
// The returned slice should be treated as read-only.
func KnownCodes() []Code {
	return knownCodes
}
