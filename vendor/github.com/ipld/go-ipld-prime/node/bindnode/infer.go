package bindnode

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/schema"
)

var (
	goTypeBool    = reflect.TypeOf(false)
	goTypeInt     = reflect.TypeOf(int(0))
	goTypeFloat   = reflect.TypeOf(0.0)
	goTypeString  = reflect.TypeOf("")
	goTypeBytes   = reflect.TypeOf([]byte{})
	goTypeLink    = reflect.TypeOf((*datamodel.Link)(nil)).Elem()
	goTypeNode    = reflect.TypeOf((*datamodel.Node)(nil)).Elem()
	goTypeCidLink = reflect.TypeOf((*cidlink.Link)(nil)).Elem()
	goTypeCid     = reflect.TypeOf((*cid.Cid)(nil)).Elem()

	schemaTypeBool   = schema.SpawnBool("Bool")
	schemaTypeInt    = schema.SpawnInt("Int")
	schemaTypeFloat  = schema.SpawnFloat("Float")
	schemaTypeString = schema.SpawnString("String")
	schemaTypeBytes  = schema.SpawnBytes("Bytes")
	schemaTypeLink   = schema.SpawnLink("Link")
	schemaTypeAny    = schema.SpawnAny("Any")
)

// Consider exposing these APIs later, if they might be useful.

type seenEntry struct {
	goType     reflect.Type
	schemaType schema.Type
}

func verifyCompatibility(seen map[seenEntry]bool, goType reflect.Type, schemaType schema.Type) {
	// TODO(mvdan): support **T as well?
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	// Avoid endless loops.
	//
	// TODO(mvdan): this is easy but fairly allocation-happy.
	// Plus, one map per call means we don't reuse work.
	if seen[seenEntry{goType, schemaType}] {
		return
	}
	seen[seenEntry{goType, schemaType}] = true

	doPanic := func(format string, args ...interface{}) {
		panicFormat := "bindnode: schema type %s is not compatible with Go type %s"
		panicArgs := []interface{}{schemaType.Name(), goType.String()}

		if format != "" {
			panicFormat += ": " + format
		}
		panicArgs = append(panicArgs, args...)
		panic(fmt.Sprintf(panicFormat, panicArgs...))
	}
	switch schemaType := schemaType.(type) {
	case *schema.TypeBool:
		if goType.Kind() != reflect.Bool {
			doPanic("kind mismatch; need boolean")
		}
	case *schema.TypeInt:
		if kind := goType.Kind(); !kindInt[kind] && !kindUint[kind] {
			doPanic("kind mismatch; need integer")
		}
	case *schema.TypeFloat:
		switch goType.Kind() {
		case reflect.Float32, reflect.Float64:
		default:
			doPanic("kind mismatch; need float")
		}
	case *schema.TypeString:
		// TODO: allow []byte?
		if goType.Kind() != reflect.String {
			doPanic("kind mismatch; need string")
		}
	case *schema.TypeBytes:
		// TODO: allow string?
		if goType.Kind() != reflect.Slice {
			doPanic("kind mismatch; need slice of bytes")
		}
		if goType.Elem().Kind() != reflect.Uint8 {
			doPanic("kind mismatch; need slice of bytes")
		}
	case *schema.TypeEnum:
		if _, ok := schemaType.RepresentationStrategy().(schema.EnumRepresentation_Int); ok {
			if kind := goType.Kind(); kind != reflect.String && !kindInt[kind] && !kindUint[kind] {
				doPanic("kind mismatch; need string or integer")
			}
		} else {
			if goType.Kind() != reflect.String {
				doPanic("kind mismatch; need string")
			}
		}
	case *schema.TypeList:
		if goType.Kind() != reflect.Slice {
			doPanic("kind mismatch; need slice")
		}
		goType = goType.Elem()
		if schemaType.ValueIsNullable() {
			if goType.Kind() != reflect.Ptr {
				doPanic("nullable types must be pointers")
			}
			goType = goType.Elem()
		}
		verifyCompatibility(seen, goType, schemaType.ValueType())
	case *schema.TypeMap:
		//	struct {
		//		Keys   []K
		//		Values map[K]V
		//	}
		if goType.Kind() != reflect.Struct {
			doPanic("kind mismatch; need struct{Keys []K; Values map[K]V}")
		}
		if goType.NumField() != 2 {
			doPanic("%d vs 2 fields", goType.NumField())
		}

		fieldKeys := goType.Field(0)
		if fieldKeys.Type.Kind() != reflect.Slice {
			doPanic("kind mismatch; need struct{Keys []K; Values map[K]V}")
		}
		verifyCompatibility(seen, fieldKeys.Type.Elem(), schemaType.KeyType())

		fieldValues := goType.Field(1)
		if fieldValues.Type.Kind() != reflect.Map {
			doPanic("kind mismatch; need struct{Keys []K; Values map[K]V}")
		}
		keyType := fieldValues.Type.Key()
		verifyCompatibility(seen, keyType, schemaType.KeyType())

		elemType := fieldValues.Type.Elem()
		if schemaType.ValueIsNullable() {
			if elemType.Kind() != reflect.Ptr {
				doPanic("nullable types must be pointers")
			}
			elemType = elemType.Elem()
		}
		verifyCompatibility(seen, elemType, schemaType.ValueType())
	case *schema.TypeStruct:
		if goType.Kind() != reflect.Struct {
			doPanic("kind mismatch; need struct")
		}

		schemaFields := schemaType.Fields()
		if goType.NumField() != len(schemaFields) {
			doPanic("%d vs %d fields", goType.NumField(), len(schemaFields))
		}
		for i, schemaField := range schemaFields {
			schemaType := schemaField.Type()
			goType := goType.Field(i).Type
			// TODO: allow "is nilable" to some degree?
			if schemaField.IsNullable() {
				if goType.Kind() != reflect.Ptr {
					doPanic("nullable types must be pointers")
				}
				goType = goType.Elem()
			}
			if schemaField.IsOptional() {
				if goType.Kind() != reflect.Ptr {
					doPanic("optional types must be pointers")
				}
				goType = goType.Elem()
			}
			verifyCompatibility(seen, goType, schemaType)
		}
	case *schema.TypeUnion:
		if goType.Kind() != reflect.Struct {
			doPanic("kind mismatch; need struct for an union")
		}

		schemaMembers := schemaType.Members()
		if goType.NumField() != len(schemaMembers) {
			doPanic("%d vs %d members", goType.NumField(), len(schemaMembers))
		}

		for i, schemaType := range schemaMembers {
			goType := goType.Field(i).Type
			if goType.Kind() != reflect.Ptr {
				doPanic("union members must be pointers")
			}
			goType = goType.Elem()
			verifyCompatibility(seen, goType, schemaType)
		}
	case *schema.TypeLink:
		if goType != goTypeLink && goType != goTypeCidLink && goType != goTypeCid {
			doPanic("links in Go must be datamodel.Link, cidlink.Link, or cid.Cid")
		}
	case *schema.TypeAny:
		// TODO: support some other option for Any, such as deferred decode
		if goType != goTypeNode {
			doPanic("Any in Go must be datamodel.Node")
		}
	default:
		panic(fmt.Sprintf("%T", schemaType))
	}
}

func inferGoType(typ schema.Type) reflect.Type {
	switch typ := typ.(type) {
	case *schema.TypeBool:
		return goTypeBool
	case *schema.TypeInt:
		return goTypeInt
	case *schema.TypeFloat:
		return goTypeFloat
	case *schema.TypeString:
		return goTypeString
	case *schema.TypeBytes:
		return goTypeBytes
	case *schema.TypeStruct:
		fields := typ.Fields()
		fieldsGo := make([]reflect.StructField, len(fields))
		for i, field := range fields {
			ftypGo := inferGoType(field.Type())
			if field.IsNullable() {
				ftypGo = reflect.PtrTo(ftypGo)
			}
			if field.IsOptional() {
				ftypGo = reflect.PtrTo(ftypGo)
			}
			fieldsGo[i] = reflect.StructField{
				Name: fieldNameFromSchema(field.Name()),
				Type: ftypGo,
			}
		}
		return reflect.StructOf(fieldsGo)
	case *schema.TypeMap:
		ktyp := inferGoType(typ.KeyType())
		vtyp := inferGoType(typ.ValueType())
		if typ.ValueIsNullable() {
			vtyp = reflect.PtrTo(vtyp)
		}
		// We need an extra field to keep the map ordered,
		// since IPLD maps must have stable iteration order.
		// We could sort when iterating, but that's expensive.
		// Keeping the insertion order is easy and intuitive.
		//
		//	struct {
		//		Keys   []K
		//		Values map[K]V
		//	}
		fieldsGo := []reflect.StructField{
			{
				Name: "Keys",
				Type: reflect.SliceOf(ktyp),
			},
			{
				Name: "Values",
				Type: reflect.MapOf(ktyp, vtyp),
			},
		}
		return reflect.StructOf(fieldsGo)
	case *schema.TypeList:
		etyp := inferGoType(typ.ValueType())
		if typ.ValueIsNullable() {
			etyp = reflect.PtrTo(etyp)
		}
		return reflect.SliceOf(etyp)
	case *schema.TypeUnion:
		// type goUnion struct {
		// 	Type1 *Type1
		// 	Type2 *Type2
		// 	...
		// }
		members := typ.Members()
		fieldsGo := make([]reflect.StructField, len(members))
		for i, ftyp := range members {
			ftypGo := inferGoType(ftyp)
			fieldsGo[i] = reflect.StructField{
				Name: fieldNameFromSchema(ftyp.Name()),
				Type: reflect.PtrTo(ftypGo),
			}
		}
		return reflect.StructOf(fieldsGo)
	case *schema.TypeLink:
		return goTypeLink
	case *schema.TypeEnum:
		// TODO: generate int for int reprs by default?
		return goTypeString
	case *schema.TypeAny:
		return goTypeNode
	}
	panic(fmt.Sprintf("%T", typ))
}

// from IPLD Schema field names like "foo" to Go field names like "Foo".
func fieldNameFromSchema(name string) string {
	return strings.Title(name)
}

var defaultTypeSystem schema.TypeSystem

func init() {
	defaultTypeSystem.Init()

	defaultTypeSystem.Accumulate(schemaTypeBool)
	defaultTypeSystem.Accumulate(schemaTypeInt)
	defaultTypeSystem.Accumulate(schemaTypeFloat)
	defaultTypeSystem.Accumulate(schemaTypeString)
	defaultTypeSystem.Accumulate(schemaTypeBytes)
	defaultTypeSystem.Accumulate(schemaTypeLink)
	defaultTypeSystem.Accumulate(schemaTypeAny)
}

// TODO: support IPLD maps and unions in inferSchema

// TODO: support bringing your own TypeSystem?

// TODO: we should probably avoid re-spawning the same types if the TypeSystem
// has them, and test that that works as expected

func inferSchema(typ reflect.Type) schema.Type {
	// TODO: support Link and Any
	switch typ.Kind() {
	case reflect.Bool:
		return schemaTypeBool
	case reflect.Int64:
		return schemaTypeInt
	case reflect.Float64:
		return schemaTypeFloat
	case reflect.String:
		return schemaTypeString
	case reflect.Struct:
		fieldsSchema := make([]schema.StructField, typ.NumField())
		for i := range fieldsSchema {
			field := typ.Field(i)
			ftyp := field.Type
			ftypSchema := inferSchema(ftyp)
			fieldsSchema[i] = schema.SpawnStructField(
				field.Name, // TODO: allow configuring the name with tags
				ftypSchema.Name(),

				// TODO: support nullable/optional with tags
				false,
				false,
			)
		}
		name := typ.Name()
		if name == "" {
			panic("TODO: anonymous composite types")
		}
		typSchema := schema.SpawnStruct(name, fieldsSchema, nil)
		defaultTypeSystem.Accumulate(typSchema)
		return typSchema
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			// Special case for []byte.
			return schemaTypeBytes
		}

		etyp := typ.Elem()
		nullable := false
		if etyp.Kind() == reflect.Ptr {
			etyp = etyp.Elem()
			nullable = true
		}
		etypSchema := inferSchema(typ.Elem())
		name := typ.Name()
		if name == "" {
			name = "List_" + etypSchema.Name()
		}
		typSchema := schema.SpawnList(name, etypSchema.Name(), nullable)
		defaultTypeSystem.Accumulate(typSchema)
		return typSchema
	}
	panic(fmt.Sprintf("%s", typ.Kind()))
}

// There are currently 27 reflect.Kind iota values,
// so 32 should be plenty to ensure we don't panic in practice.

var kindInt = [32]bool{
	reflect.Int:   true,
	reflect.Int8:  true,
	reflect.Int16: true,
	reflect.Int32: true,
	reflect.Int64: true,
}

var kindUint = [32]bool{
	reflect.Uint:   true,
	reflect.Uint8:  true,
	reflect.Uint16: true,
	reflect.Uint32: true,
	reflect.Uint64: true,
}
