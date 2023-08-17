// Copyright The OpenTelemetry Authors
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

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/extension/experimental/storage"
)

// persistentQueue holds the queue backed by file storage
type persistentQueue struct {
	logger     *zap.Logger
	stopWG     sync.WaitGroup
	stopOnce   sync.Once
	stopChan   chan struct{}
	numWorkers int
	storage    *persistentContiguousStorage
}

// buildPersistentStorageName returns a name that is constructed out of queue name and signal type. This is done
// to avoid conflicts between different signals, which require unique persistent storage name
func buildPersistentStorageName(name string, signal config.DataType) string {
	return fmt.Sprintf("%s-%s", name, signal)
}

// NewPersistentQueue creates a new queue backed by file storage; name and signal must be a unique combination that identifies the queue storage
func NewPersistentQueue(ctx context.Context, name string, signal config.DataType, capacity int, logger *zap.Logger, client storage.Client, unmarshaler RequestUnmarshaler) ProducerConsumerQueue {
	return &persistentQueue{
		logger:   logger,
		stopChan: make(chan struct{}),
		storage:  newPersistentContiguousStorage(ctx, buildPersistentStorageName(name, signal), uint64(capacity), logger, client, unmarshaler),
	}
}

// StartConsumers starts the given number of consumers which will be consuming items
func (pq *persistentQueue) StartConsumers(num int, callback func(item Request)) {
	pq.numWorkers = num

	for i := 0; i < pq.numWorkers; i++ {
		pq.stopWG.Add(1)
		go func() {
			defer pq.stopWG.Done()

			for {
				select {
				case req := <-pq.storage.get():
					callback(req)
				case <-pq.stopChan:
					return
				}
			}
		}()
	}
}

// Produce adds an item to the queue and returns true if it was accepted
func (pq *persistentQueue) Produce(item Request) bool {
	err := pq.storage.put(item)
	return err == nil
}

// Stop stops accepting items, shuts down the queue and closes the persistent queue
func (pq *persistentQueue) Stop() {
	pq.stopOnce.Do(func() {
		pq.storage.stop()
		close(pq.stopChan)
		pq.stopWG.Wait()
	})
}

// Size returns the current depth of the queue, excluding the item already in the storage channel (if any)
func (pq *persistentQueue) Size() int {
	return int(pq.storage.size())
}
