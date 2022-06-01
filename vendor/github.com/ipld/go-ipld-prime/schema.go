package ipld

import (
	"bytes"
	"io"
	"os"

	"github.com/ipld/go-ipld-prime/schema"
	schemadmt "github.com/ipld/go-ipld-prime/schema/dmt"
	schemadsl "github.com/ipld/go-ipld-prime/schema/dsl"
)

// LoadSchemaBytes is a shortcut for LoadSchema for the common case where
// the schema is available as a buffer or a string, such as via go:embed.
func LoadSchemaBytes(src []byte) (*schema.TypeSystem, error) {
	return LoadSchema("", bytes.NewReader(src))
}

// LoadSchemaBytes is a shortcut for LoadSchema for the common case where
// the schema is a file on disk.
func LoadSchemaFile(path string) (*schema.TypeSystem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadSchema(path, f)
}

// LoadSchema parses an IPLD Schema in its DSL form
// and compiles its types into a standalone TypeSystem.
func LoadSchema(name string, r io.Reader) (*schema.TypeSystem, error) {
	sch, err := schemadsl.Parse(name, r)
	if err != nil {
		return nil, err
	}
	ts := new(schema.TypeSystem)
	ts.Init()
	if err := schemadmt.Compile(ts, sch); err != nil {
		return nil, err
	}
	return ts, nil
}
