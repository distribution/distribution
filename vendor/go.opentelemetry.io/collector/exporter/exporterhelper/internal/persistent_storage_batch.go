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
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"go.uber.org/zap"

	"go.opentelemetry.io/collector/extension/experimental/storage"
)

var errItemIndexArrInvalidDataType = errors.New("invalid data type, expected []itemIndex")

// batchStruct provides convenience capabilities for creating and processing storage extension batches
type batchStruct struct {
	logger *zap.Logger
	pcs    *persistentContiguousStorage

	operations    []storage.Operation
	getOperations map[string]storage.Operation
}

func newBatch(pcs *persistentContiguousStorage) *batchStruct {
	return &batchStruct{
		logger:        pcs.logger,
		pcs:           pcs,
		operations:    []storage.Operation{},
		getOperations: map[string]storage.Operation{},
	}
}

// execute runs the provided operations in order
func (bof *batchStruct) execute(ctx context.Context) (*batchStruct, error) {
	err := bof.pcs.client.Batch(ctx, bof.operations...)
	if err != nil {
		return nil, err
	}

	return bof, nil
}

// set adds a Set operation to the batch
func (bof *batchStruct) set(key string, value interface{}, marshal func(interface{}) ([]byte, error)) *batchStruct {
	valueBytes, err := marshal(value)
	if err != nil {
		bof.logger.Debug("Failed marshaling item, skipping it", zap.String(zapKey, key), zap.Error(err))
	} else {
		bof.operations = append(bof.operations, storage.SetOperation(key, valueBytes))
	}

	return bof
}

// get adds a Get operation to the batch. After executing, its result will be available through getResult
func (bof *batchStruct) get(keys ...string) *batchStruct {
	for _, key := range keys {
		op := storage.GetOperation(key)
		bof.getOperations[key] = op
		bof.operations = append(bof.operations, op)
	}

	return bof
}

// delete adds a Delete operation to the batch
func (bof *batchStruct) delete(keys ...string) *batchStruct {
	for _, key := range keys {
		bof.operations = append(bof.operations, storage.DeleteOperation(key))
	}

	return bof
}

// getResult returns the result of a Get operation for a given key using the provided unmarshal function.
// It should be called after execute. It may return nil value
func (bof *batchStruct) getResult(key string, unmarshal func([]byte) (interface{}, error)) (interface{}, error) {
	op := bof.getOperations[key]
	if op == nil {
		return nil, errKeyNotPresentInBatch
	}

	if op.Value == nil {
		return nil, nil
	}

	return unmarshal(op.Value)
}

// getRequestResult returns the result of a Get operation as a request
// If the value cannot be retrieved, it returns an error
func (bof *batchStruct) getRequestResult(key string) (Request, error) {
	reqIf, err := bof.getResult(key, bof.bytesToRequest)
	if err != nil {
		return nil, err
	}
	if reqIf == nil {
		return nil, errValueNotSet
	}

	return reqIf.(Request), nil
}

// getItemIndexResult returns the result of a Get operation as an itemIndex
// If the value cannot be retrieved, it returns an error
func (bof *batchStruct) getItemIndexResult(key string) (itemIndex, error) {
	itemIndexIf, err := bof.getResult(key, bytesToItemIndex)
	if err != nil {
		return itemIndex(0), err
	}

	if itemIndexIf == nil {
		return itemIndex(0), errValueNotSet
	}

	return itemIndexIf.(itemIndex), nil
}

// getItemIndexArrayResult returns the result of a Get operation as a itemIndexArray
// It may return nil value
func (bof *batchStruct) getItemIndexArrayResult(key string) ([]itemIndex, error) {
	itemIndexArrIf, err := bof.getResult(key, bytesToItemIndexArray)
	if err != nil {
		return nil, err
	}

	if itemIndexArrIf == nil {
		return nil, nil
	}

	return itemIndexArrIf.([]itemIndex), nil
}

// setRequest adds Set operation over a given request to the batch
func (bof *batchStruct) setRequest(key string, value Request) *batchStruct {
	return bof.set(key, value, requestToBytes)
}

// setItemIndex adds Set operation over a given itemIndex to the batch
func (bof *batchStruct) setItemIndex(key string, value itemIndex) *batchStruct {
	return bof.set(key, value, itemIndexToBytes)
}

// setItemIndexArray adds Set operation over a given itemIndex array to the batch
func (bof *batchStruct) setItemIndexArray(key string, value []itemIndex) *batchStruct {
	return bof.set(key, value, itemIndexArrayToBytes)
}

func itemIndexToBytes(val interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.LittleEndian, val)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

func bytesToItemIndex(b []byte) (interface{}, error) {
	var val itemIndex
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &val)
	if err != nil {
		return val, err
	}
	return val, nil
}

func itemIndexArrayToBytes(arr interface{}) ([]byte, error) {
	var buf bytes.Buffer
	size := 0

	if arr != nil {
		arrItemIndex, ok := arr.([]itemIndex)
		if ok {
			size = len(arrItemIndex)
		} else {
			return nil, errItemIndexArrInvalidDataType
		}
	}

	err := binary.Write(&buf, binary.LittleEndian, uint32(size))
	if err != nil {
		return nil, err
	}

	err = binary.Write(&buf, binary.LittleEndian, arr)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

func bytesToItemIndexArray(b []byte) (interface{}, error) {
	var size uint32
	reader := bytes.NewReader(b)
	err := binary.Read(reader, binary.LittleEndian, &size)
	if err != nil {
		return nil, err
	}

	val := make([]itemIndex, size)
	err = binary.Read(reader, binary.LittleEndian, &val)
	return val, err
}

func requestToBytes(req interface{}) ([]byte, error) {
	return req.(Request).Marshal()
}

func (bof *batchStruct) bytesToRequest(b []byte) (interface{}, error) {
	return bof.pcs.unmarshaler(b)
}
