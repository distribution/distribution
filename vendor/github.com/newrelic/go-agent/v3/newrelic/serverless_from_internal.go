// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// parseServerlessPayload exists for testing.
func parseServerlessPayload(data []byte) (metadata, uncompressedData map[string]json.RawMessage, err error) {
	var arr [4]json.RawMessage
	if err = json.Unmarshal(data, &arr); nil != err {
		err = fmt.Errorf("unable to unmarshal serverless data array: %v", err)
		return
	}
	var dataJSON []byte
	compressed := strings.Trim(string(arr[3]), `"`)
	if dataJSON, err = decodeUncompress(compressed); nil != err {
		err = fmt.Errorf("unable to uncompress serverless data: %v", err)
		return
	}
	if err = json.Unmarshal(dataJSON, &uncompressedData); nil != err {
		err = fmt.Errorf("unable to unmarshal uncompressed serverless data: %v", err)
		return
	}
	if err = json.Unmarshal(arr[2], &metadata); nil != err {
		err = fmt.Errorf("unable to unmarshal serverless metadata: %v", err)
		return
	}
	return
}

func decodeUncompress(input string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if nil != err {
		return nil, err
	}

	buf := bytes.NewBuffer(decoded)
	gz, err := gzip.NewReader(buf)
	if nil != err {
		return nil, err
	}
	var out bytes.Buffer
	io.Copy(&out, gz)
	gz.Close()

	return out.Bytes(), nil
}
