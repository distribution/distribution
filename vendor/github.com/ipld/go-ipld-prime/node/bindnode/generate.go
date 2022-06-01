package bindnode

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"strings"

	"github.com/ipld/go-ipld-prime/schema"
)

// TODO(mvdan): deduplicate with inferGoType once reflect supports creating named types

func produceGoType(goTypes map[string]string, typ schema.Type) (name, src string) {
	if typ, ok := typ.(interface{ IsAnonymous() bool }); ok {
		if typ.IsAnonymous() {
			panic("TODO: does this ever happen?")
		}
	}

	name = string(typ.Name())

	switch typ.(type) {
	case *schema.TypeBool:
		return goTypeBool.String(), ""
	case *schema.TypeInt:
		return goTypeInt.String(), ""
	case *schema.TypeFloat:
		return goTypeFloat.String(), ""
	case *schema.TypeString:
		return goTypeString.String(), ""
	case *schema.TypeBytes:
		return goTypeBytes.String(), ""
	case *schema.TypeLink:
		return goTypeLink.String(), "" // datamodel.Link
	case *schema.TypeAny:
		return goTypeNode.String(), "" // datamodel.Node
	}

	// Results are cached in goTypes.
	if src := goTypes[name]; src != "" {
		return name, src
	}

	src = produceGoTypeInner(goTypes, name, typ)
	goTypes[name] = src
	return name, src
}

func produceGoTypeInner(goTypes map[string]string, name string, typ schema.Type) (src string) {
	// Avoid infinite cycles.
	// produceGoType will fill in the final type later.
	goTypes[name] = "WIP"

	switch typ := typ.(type) {
	case *schema.TypeEnum:
		// TODO: also generate named constants for the members.
		return goTypeString.String()
	case *schema.TypeStruct:
		var b strings.Builder
		fmt.Fprintf(&b, "struct {\n")
		fields := typ.Fields()
		for _, field := range fields {
			fmt.Fprintf(&b, "%s ", fieldNameFromSchema(field.Name()))
			ftypGo, _ := produceGoType(goTypes, field.Type())
			if field.IsNullable() {
				fmt.Fprintf(&b, "*")
			}
			if field.IsOptional() {
				fmt.Fprintf(&b, "*")
			}
			fmt.Fprintf(&b, "%s\n", ftypGo)
		}
		fmt.Fprintf(&b, "\n}")
		return b.String()
	case *schema.TypeMap:
		ktyp, _ := produceGoType(goTypes, typ.KeyType())
		vtyp, _ := produceGoType(goTypes, typ.ValueType())
		if typ.ValueIsNullable() {
			vtyp = "*" + vtyp
		}
		return fmt.Sprintf(`struct {
			Keys []%s
			Values map[%s]%s
		}`, ktyp, ktyp, vtyp)
	case *schema.TypeList:
		etyp, _ := produceGoType(goTypes, typ.ValueType())
		if typ.ValueIsNullable() {
			etyp = "*" + etyp
		}
		return fmt.Sprintf("[]%s", etyp)
	case *schema.TypeUnion:
		var b strings.Builder
		fmt.Fprintf(&b, "struct{\n")
		members := typ.Members()
		for _, ftyp := range members {
			ftypGo, _ := produceGoType(goTypes, ftyp)
			fmt.Fprintf(&b, "%s ", fieldNameFromSchema(string(ftyp.Name())))
			fmt.Fprintf(&b, "*%s\n", ftypGo)
		}
		fmt.Fprintf(&b, "\n}")
		return b.String()
	}
	panic(fmt.Sprintf("%T\n", typ))
}

// ProduceGoTypes infers Go types from an IPLD schema in ts
// and writes their Go source code type declarations to w.
// Note that just the types are written,
// without a package declaration nor any imports.
//
// This gives a good starting point when wanting to use bindnode with Go types,
// but users will generally want to own and modify the types afterward,
// so they can add documentation or tweak the types as needed.
func ProduceGoTypes(w io.Writer, ts *schema.TypeSystem) error {
	goTypes := make(map[string]string)
	var buf bytes.Buffer
	for _, name := range ts.Names() {
		schemaType := ts.TypeByName(string(name))
		if name != schemaType.Name() {
			panic(fmt.Sprintf("%s vs %s", name, schemaType.Name()))
		}
		_, src := produceGoType(goTypes, schemaType)
		if src == "" {
			continue // scalar type used directly
		}
		fmt.Fprintf(&buf, "type %s %s\n", name, src)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	if _, err := w.Write(src); err != nil {
		return err
	}
	return nil
}
