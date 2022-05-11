package builder

import (
	"errors"
	"strconv"
	"time"

	"github.com/ipfs/go-unixfsnode/data"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/fluent/qp"
)

// BuildUnixFS provides a clean, validated interface to building data structures
// that match the UnixFS protobuf encoded in the Data member of a ProtoNode
// with sensible defaults
//
//   smallFileData, err := BuildUnixFS(func(b *Builder) {
//      Data(b, []byte{"hello world"})
//      Mtime(b, func(tb TimeBuilder) {
//				Time(tb, time.Now())
//			})
//   })
//
func BuildUnixFS(fn func(*Builder)) (data.UnixFSData, error) {
	nd, err := qp.BuildMap(data.Type.UnixFSData, -1, func(ma ipld.MapAssembler) {
		b := &Builder{MapAssembler: ma}
		fn(b)
		if !b.hasBlockSizes {
			qp.MapEntry(ma, data.Field__BlockSizes, qp.List(0, func(ipld.ListAssembler) {}))
		}
		if !b.hasDataType {
			qp.MapEntry(ma, data.Field__DataType, qp.Int(data.Data_File))
		}
	})
	if err != nil {
		return nil, err
	}
	return nd.(data.UnixFSData), nil
}

// Builder is an interface for making UnixFS data nodes
type Builder struct {
	ipld.MapAssembler
	hasDataType   bool
	hasBlockSizes bool
}

// DataType sets the default on a builder for a UnixFS node - default is File
func DataType(b *Builder, dataType int64) {
	_, ok := data.DataTypeNames[dataType]
	if !ok {
		panic(data.ErrInvalidDataType{DataType: dataType})
	}
	qp.MapEntry(b.MapAssembler, data.Field__DataType, qp.Int(dataType))
	b.hasDataType = true
}

// Data sets the data member inside the UnixFS data
func Data(b *Builder, dataBytes []byte) {
	qp.MapEntry(b.MapAssembler, data.Field__Data, qp.Bytes(dataBytes))
}

// FileSize sets the file size which should be the size of actual bytes underneath
// this node for large files, w/o additional bytes to encode intermediate nodes
func FileSize(b *Builder, fileSize uint64) {
	qp.MapEntry(b.MapAssembler, data.Field__FileSize, qp.Int(int64(fileSize)))
}

// BlockSizes encodes block sizes for each child node
func BlockSizes(b *Builder, blockSizes []uint64) {
	qp.MapEntry(b.MapAssembler, data.Field__BlockSizes, qp.List(int64(len(blockSizes)), func(la ipld.ListAssembler) {
		for _, bs := range blockSizes {
			qp.ListEntry(la, qp.Int(int64(bs)))
		}
	}))
	b.hasBlockSizes = true
}

// HashType sets the hash function for this node -- only applicable to HAMT
func HashType(b *Builder, hashType uint64) {
	qp.MapEntry(b.MapAssembler, data.Field__HashType, qp.Int(int64(hashType)))
}

// Fanout sets the fanout in a HAMT tree
func Fanout(b *Builder, fanout uint64) {
	qp.MapEntry(b.MapAssembler, data.Field__Fanout, qp.Int(int64(fanout)))
}

// Permissions sets file permissions for the Mode member of the UnixFS node
func Permissions(b *Builder, mode int) {
	mode = mode & 0xFFF
	qp.MapEntry(b.MapAssembler, data.Field__Mode, qp.Int(int64(mode)))
}

func parseModeString(modeString string) (uint64, error) {
	if len(modeString) > 0 && modeString[0] == '0' {
		return strconv.ParseUint(modeString, 8, 32)
	}
	return strconv.ParseUint(modeString, 10, 32)
}

// PermissionsString sets file permissions for the Mode member of the UnixFS node,
// parsed from a typical octect encoded permission string (eg '0755')
func PermissionsString(b *Builder, modeString string) {
	mode64, err := parseModeString(modeString)
	if err != nil {
		panic(err)
	}
	mode64 = mode64 & 0xFFF
	qp.MapEntry(b.MapAssembler, data.Field__Mode, qp.Int(int64(mode64)))
}

// Mtime sets the modification time for this node using the time builder interface
// and associated methods
func Mtime(b *Builder, fn func(tb TimeBuilder)) {
	qp.MapEntry(b.MapAssembler, data.Field__Mtime, qp.Map(-1, func(ma ipld.MapAssembler) {
		fn(ma)
	}))
}

// TimeBuilder is a simple interface for constructing the time member of UnixFS data
type TimeBuilder ipld.MapAssembler

// Time sets the modification time from a golang time value
func Time(ma TimeBuilder, t time.Time) {
	Seconds(ma, t.Unix())
	FractionalNanoseconds(ma, int32(t.Nanosecond()))
}

// Seconds sets the seconds for a modification time
func Seconds(ma TimeBuilder, seconds int64) {
	qp.MapEntry(ma, data.Field__Seconds, qp.Int(seconds))

}

// FractionalNanoseconds sets the nanoseconds for a modification time (must
// be between 0 & a billion)
func FractionalNanoseconds(ma TimeBuilder, nanoseconds int32) {
	if nanoseconds < 0 || nanoseconds > 999999999 {
		panic(errors.New("mtime-nsecs must be within the range [0,999999999]"))
	}
	qp.MapEntry(ma, data.Field__Nanoseconds, qp.Int(int64(nanoseconds)))
}
