package data

import "google.golang.org/protobuf/encoding/protowire"

const (
	Data_DataTypeWireNum                  protowire.Number = 1
	Data_DataWireNum                      protowire.Number = 2
	Data_FileSizeWireNum                  protowire.Number = 3
	Data_BlockSizesWireNum                protowire.Number = 4
	Data_HashTypeWireNum                  protowire.Number = 5
	Data_FanoutWireNum                    protowire.Number = 6
	Data_ModeWireNum                      protowire.Number = 7
	Data_MtimeWireNum                     protowire.Number = 8
	UnixTime_SecondsWireNum               protowire.Number = 1
	UnixTime_FractionalNanosecondsWireNum protowire.Number = 2
	Metadata_MimeTypeWireNum              protowire.Number = 1
)
