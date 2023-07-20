// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"net/http"

	"github.com/newrelic/go-agent/v3/internal/cat"
)

// InboundHTTPRequest adds the inbound request metadata to the txnCrossProcess.
func (txp *txnCrossProcess) InboundHTTPRequest(hdr http.Header) error {
	return txp.handleInboundRequestHeaders(httpHeaderToMetadata(hdr))
}

// appDataToHTTPHeader encapsulates the given appData value in the correct HTTP
// header.
func appDataToHTTPHeader(appData string) http.Header {
	header := http.Header{}

	if appData != "" {
		header.Add(cat.NewRelicAppDataName, appData)
	}

	return header
}

// httpHeaderToAppData gets the appData value from the correct HTTP header.
func httpHeaderToAppData(header http.Header) string {
	if header == nil {
		return ""
	}

	return header.Get(cat.NewRelicAppDataName)
}

// httpHeaderToMetadata gets the cross process metadata from the relevant HTTP
// headers.
func httpHeaderToMetadata(header http.Header) crossProcessMetadata {
	if header == nil {
		return crossProcessMetadata{}
	}

	return crossProcessMetadata{
		ID:         header.Get(cat.NewRelicIDName),
		TxnData:    header.Get(cat.NewRelicTxnName),
		Synthetics: header.Get(cat.NewRelicSyntheticsName),
	}
}

// metadataToHTTPHeader creates a set of HTTP headers to represent the given
// cross process metadata.
func metadataToHTTPHeader(metadata crossProcessMetadata) http.Header {
	header := http.Header{}

	if metadata.ID != "" {
		header.Add(cat.NewRelicIDName, metadata.ID)
	}

	if metadata.TxnData != "" {
		header.Add(cat.NewRelicTxnName, metadata.TxnData)
	}

	if metadata.Synthetics != "" {
		header.Add(cat.NewRelicSyntheticsName, metadata.Synthetics)
	}

	return header
}
