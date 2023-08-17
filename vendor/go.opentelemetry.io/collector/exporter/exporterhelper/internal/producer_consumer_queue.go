// Copyright The OpenTelemetry Authors
// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal // import "go.opentelemetry.io/collector/exporter/exporterhelper/internal"

// ProducerConsumerQueue defines a producer-consumer exchange which can be backed by e.g. the memory-based ring buffer queue
// (boundedMemoryQueue) or via a disk-based queue (persistentQueue)
type ProducerConsumerQueue interface {
	// StartConsumers starts a given number of goroutines consuming items from the queue
	// and passing them into the consumer callback.
	StartConsumers(num int, callback func(item Request))
	// Produce is used by the producer to submit new item to the queue. Returns false if the item wasn't added
	// to the queue due to queue overflow.
	Produce(item Request) bool
	// Size returns the current Size of the queue
	Size() int
	// Stop stops all consumers, as well as the length reporter if started,
	// and releases the items channel. It blocks until all consumers have stopped.
	Stop()
}
