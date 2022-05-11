package data

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

type ErrWrongNodeType struct {
	Expected int64
	Actual   int64
}

func (e ErrWrongNodeType) Error() string {
	expectedName, ok := DataTypeNames[e.Expected]
	if !ok {
		expectedName = "Unknown Type"
	}
	actualName, ok := DataTypeNames[e.Actual]
	if !ok {
		actualName = "Unknown Type"
	}
	return fmt.Sprintf("incorrect Node Type: (UnixFSData) expected type: %s, actual type: %s", expectedName, actualName)
}

type ErrWrongWireType struct {
	Module   string
	Field    string
	Expected protowire.Type
	Actual   protowire.Type
}

func (e ErrWrongWireType) Error() string {
	return fmt.Sprintf("protobuf: (%s) invalid wireType, field: %s, expected %d, got %d", e.Module, e.Field, e.Expected, e.Actual)
}

type ErrInvalidDataType struct {
	DataType int64
}

func (e ErrInvalidDataType) Error() string {
	return fmt.Sprintf("type: %d is not valid", e.DataType)
}
