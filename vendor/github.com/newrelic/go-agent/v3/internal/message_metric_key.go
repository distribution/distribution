// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

// MessageMetricKey is the key to use for message segments.
type MessageMetricKey struct {
	Library         string
	DestinationType string
	Consumer        bool
	DestinationName string
	DestinationTemp bool
}

// Name returns the metric name value for this MessageMetricKey to be used for
// scoped and unscoped metrics.
//
// Producers
// MessageBroker/{Library}/{Destination Type}/{Action}/Named/{Destination Name}
// MessageBroker/{Library}/{Destination Type}/{Action}/Temp
//
// Consumers
// OtherTransaction/Message/{Library}/{DestinationType}/Named/{Destination Name}
// OtherTransaction/Message/{Library}/{DestinationType}/Temp
func (key MessageMetricKey) Name() string {
	var destination string
	if key.DestinationTemp {
		destination = "Temp"
	} else if key.DestinationName == "" {
		destination = "Named/Unknown"
	} else {
		destination = "Named/" + key.DestinationName
	}

	if key.Consumer {
		return "Message/" + key.Library +
			"/" + key.DestinationType +
			"/" + destination
	}
	return "MessageBroker/" + key.Library +
		"/" + key.DestinationType +
		"/Produce/" + destination
}
