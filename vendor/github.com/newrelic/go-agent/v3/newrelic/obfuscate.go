// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"encoding/base64"
	"errors"
)

// deobfuscate deobfuscates a byte array.
func deobfuscate(in string, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("key cannot be zero length")
	}

	decoded, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return nil, err
	}

	out := make([]byte, len(decoded))
	for i, c := range decoded {
		out[i] = c ^ key[i%len(key)]
	}

	return out, nil
}

// obfuscate obfuscates a byte array for transmission in CAT and RUM.
func obfuscate(in, key []byte) (string, error) {
	if len(key) == 0 {
		return "", errors.New("key cannot be zero length")
	}

	out := make([]byte, len(in))
	for i, c := range in {
		out[i] = c ^ key[i%len(key)]
	}

	return base64.StdEncoding.EncodeToString(out), nil
}
