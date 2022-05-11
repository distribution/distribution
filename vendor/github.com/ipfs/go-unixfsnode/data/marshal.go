package data

import "google.golang.org/protobuf/encoding/protowire"

// EncodeUnixFSData serializes a UnixFSData node to bytes
func EncodeUnixFSData(node UnixFSData) []byte {
	// 1KiB can be allocated on the stack, and covers most small nodes
	// without having to grow the buffer and cause allocations.
	enc := make([]byte, 0, 1024)

	return AppendEncodeUnixFSData(enc, node)
}

func AppendEncodeUnixFSData(enc []byte, node UnixFSData) []byte {
	enc = protowire.AppendTag(enc, Data_DataTypeWireNum, protowire.VarintType)
	enc = protowire.AppendVarint(enc, uint64(node.FieldDataType().Int()))
	if node.FieldData().Exists() {
		enc = protowire.AppendTag(enc, Data_DataWireNum, protowire.BytesType)
		enc = protowire.AppendBytes(enc, node.FieldData().Must().Bytes())
	}
	if node.FieldFileSize().Exists() {
		enc = protowire.AppendTag(enc, Data_FileSizeWireNum, protowire.VarintType)
		enc = protowire.AppendVarint(enc, uint64(node.FieldFileSize().Must().Int()))
	}
	itr := node.FieldBlockSizes().Iterator()
	for !itr.Done() {
		_, nd := itr.Next()
		enc = protowire.AppendTag(enc, Data_BlockSizesWireNum, protowire.VarintType)
		enc = protowire.AppendVarint(enc, uint64(nd.Int()))
	}
	if node.FieldHashType().Exists() {
		enc = protowire.AppendTag(enc, Data_HashTypeWireNum, protowire.VarintType)
		enc = protowire.AppendVarint(enc, uint64(node.FieldHashType().Must().Int()))
	}
	if node.FieldFanout().Exists() {
		enc = protowire.AppendTag(enc, Data_FanoutWireNum, protowire.VarintType)
		enc = protowire.AppendVarint(enc, uint64(node.FieldFanout().Must().Int()))
	}
	if node.FieldMode().Exists() && node.FieldMode().Must().Int() != int64(DefaultPermissions(node)) {
		enc = protowire.AppendTag(enc, Data_ModeWireNum, protowire.VarintType)
		enc = protowire.AppendVarint(enc, uint64(node.FieldMode().Must().Int()))
	}
	if node.FieldMtime().Exists() {
		mtime := node.FieldMtime().Must()
		size := 0
		size += protowire.SizeTag(1)
		size += protowire.SizeVarint(uint64(mtime.FieldSeconds().Int()))
		if mtime.FieldFractionalNanoseconds().Exists() {
			size += protowire.SizeTag(2)
			size += protowire.SizeFixed32()
		}
		enc = protowire.AppendTag(enc, Data_MtimeWireNum, protowire.BytesType)
		enc = protowire.AppendVarint(enc, uint64(size))
		enc = AppendEncodeUnixTime(enc, mtime)
	}
	return enc
}

func AppendEncodeUnixTime(enc []byte, node UnixTime) []byte {
	enc = protowire.AppendTag(enc, UnixTime_SecondsWireNum, protowire.VarintType)
	enc = protowire.AppendVarint(enc, uint64(node.FieldSeconds().Int()))
	if node.FieldFractionalNanoseconds().Exists() {
		enc = protowire.AppendTag(enc, UnixTime_FractionalNanosecondsWireNum, protowire.Fixed32Type)
		enc = protowire.AppendFixed32(enc, uint32(node.FieldFractionalNanoseconds().Must().Int()))
	}
	return enc
}

// EncodeUnixFSMetadata serializes a UnixFSMetadata node to bytes
func EncodeUnixFSMetadata(node UnixFSMetadata) []byte {
	// 1KiB can be allocated on the stack, and covers most small nodes
	// without having to grow the buffer and cause allocations.
	enc := make([]byte, 0, 1024)

	return AppendEncodeUnixFSMetadata(enc, node)
}

func AppendEncodeUnixFSMetadata(enc []byte, node UnixFSMetadata) []byte {
	if node.FieldMimeType().Exists() {
		enc = protowire.AppendTag(enc, Metadata_MimeTypeWireNum, protowire.BytesType)
		enc = protowire.AppendBytes(enc, []byte(node.FieldMimeType().Must().String()))
	}
	return enc
}
