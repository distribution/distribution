package schemadmt

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
)

// This schema follows https://ipld.io/specs/schemas/schema-schema.ipldsch.

var Type struct {
	Schema schema.TypedPrototype
}

//go:generate go run -tags=schemadmtgen gen.go

var schemaTypeSystem schema.TypeSystem

func init() {
	var ts schema.TypeSystem
	ts.Init()

	// I've elided all references to Advancedlayouts stuff for the moment.
	// (Not because it's particularly hard or problematic; I just want to draw a slightly smaller circle first.)

	// Prelude
	ts.Accumulate(schema.SpawnString("String"))
	ts.Accumulate(schema.SpawnBool("Bool"))
	ts.Accumulate(schema.SpawnInt("Int"))
	ts.Accumulate(schema.SpawnFloat("Float"))
	ts.Accumulate(schema.SpawnBytes("Bytes"))

	// Schema-schema!
	// In the same order as the spec's ipldsch file.
	// Note that ADL stuff is excluded for now, as per above.
	ts.Accumulate(schema.SpawnString("TypeName"))
	ts.Accumulate(schema.SpawnStruct("Schema",
		[]schema.StructField{
			schema.SpawnStructField("types", "Map__TypeName__TypeDefn", false, false),
			// also: `advanced AdvancedDataLayoutMap`, but as commented above, we'll pursue this later.
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnMap("Map__TypeName__TypeDefn",
		"TypeName", "TypeDefn", false,
	))
	ts.Accumulate(schema.SpawnUnion("TypeDefn",
		[]schema.TypeName{
			"TypeDefnBool",
			"TypeDefnString",
			"TypeDefnBytes",
			"TypeDefnInt",
			"TypeDefnFloat",
			"TypeDefnMap",
			"TypeDefnList",
			"TypeDefnLink",
			"TypeDefnUnion",
			"TypeDefnStruct",
			"TypeDefnEnum",
			"TypeDefnUnit",
			"TypeDefnAny",
			"TypeDefnCopy",
		},
		// TODO: spec uses inline repr.
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"bool":   "TypeDefnBool",
			"string": "TypeDefnString",
			"bytes":  "TypeDefnBytes",
			"int":    "TypeDefnInt",
			"float":  "TypeDefnFloat",
			"map":    "TypeDefnMap",
			"list":   "TypeDefnList",
			"link":   "TypeDefnLink",
			"union":  "TypeDefnUnion",
			"struct": "TypeDefnStruct",
			"enum":   "TypeDefnEnum",
			"unit":   "TypeDefnUnit",
			"any":    "TypeDefnAny",
			"copy":   "TypeDefnCopy",
		}),
	))
	ts.Accumulate(schema.SpawnUnion("TypeNameOrInlineDefn",
		[]schema.TypeName{
			"TypeName",
			"InlineDefn",
		},
		schema.SpawnUnionRepresentationKinded(map[datamodel.Kind]schema.TypeName{
			datamodel.Kind_String: "TypeName",
			datamodel.Kind_Map:    "InlineDefn",
		}),
	))
	ts.Accumulate(schema.SpawnUnion("InlineDefn",
		[]schema.TypeName{
			"TypeDefnMap",
			"TypeDefnList",
			"TypeDefnLink",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"map":  "TypeDefnMap",
			"list": "TypeDefnList",
			"link": "TypeDefnLink",
		}),
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnBool",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnString",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnBytes",
		[]schema.StructField{},
		// No BytesRepresentation, since we omit ADL stuff.
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnInt",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnFloat",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnMap",
		[]schema.StructField{
			schema.SpawnStructField("keyType", "TypeName", false, false),
			schema.SpawnStructField("valueType", "TypeNameOrInlineDefn", false, false),
			schema.SpawnStructField("valueNullable", "Bool", true, false),               // TODO: wants to use the "implicit" feature, but not supported yet
			schema.SpawnStructField("representation", "MapRepresentation", true, false), // XXXXXX
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnUnion("MapRepresentation",
		[]schema.TypeName{
			"MapRepresentation_Map",
			"MapRepresentation_Stringpairs",
			"MapRepresentation_Listpairs",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"map":         "MapRepresentation_Map",
			"stringpairs": "MapRepresentation_Stringpairs",
			"listpairs":   "MapRepresentation_Listpairs",
		}),
	))
	ts.Accumulate(schema.SpawnStruct("MapRepresentation_Map",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("MapRepresentation_Stringpairs",
		[]schema.StructField{
			schema.SpawnStructField("innerDelim", "String", false, false),
			schema.SpawnStructField("entryDelim", "String", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("MapRepresentation_Listpairs",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnList",
		[]schema.StructField{
			schema.SpawnStructField("valueType", "TypeNameOrInlineDefn", false, false),
			schema.SpawnStructField("valueNullable", "Bool", true, false),                // TODO: wants to use the "implicit" feature, but not supported yet
			schema.SpawnStructField("representation", "ListRepresentation", true, false), // XXXXXX
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnUnion("ListRepresentation",
		[]schema.TypeName{
			"ListRepresentation_List",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"list": "ListRepresentation_List",
		}),
	))
	ts.Accumulate(schema.SpawnStruct("ListRepresentation_List",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnUnion",
		[]schema.StructField{
			// n.b. we could conceivably allow TypeNameOrInlineDefn here rather than just TypeName.  but... we'd rather not: imagine what that means about the type-level behavior of the union: the name munge for the anonymous type would suddenly become load-bearing.  would rather not.
			schema.SpawnStructField("members", "List__UnionMember", false, false),
			schema.SpawnStructField("representation", "UnionRepresentation", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnList("List__UnionMember",
		"UnionMember", false,
	))
	ts.Accumulate(schema.SpawnUnion("UnionMember",
		[]schema.TypeName{
			"TypeName",
			"UnionMemberInlineDefn",
		},
		schema.SpawnUnionRepresentationKinded(map[datamodel.Kind]schema.TypeName{
			datamodel.Kind_String: "TypeName",
			datamodel.Kind_Map:    "UnionMemberInlineDefn",
		}),
	))
	ts.Accumulate(schema.SpawnUnion("UnionMemberInlineDefn",
		[]schema.TypeName{
			"TypeDefnLink",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"link": "TypeDefnLink",
		}),
	))
	ts.Accumulate(schema.SpawnList("List__TypeName", // todo: this is a slight hack: should be an anon inside TypeDefnUnion.members.
		"TypeName", false,
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnLink",
		[]schema.StructField{
			schema.SpawnStructField("expectedType", "TypeName", true, false), // todo: this uses an implicit with a value of 'any' in the schema-schema, but that's been questioned before.  maybe it should simply be an optional.
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnUnion("UnionRepresentation",
		[]schema.TypeName{
			"UnionRepresentation_Kinded",
			"UnionRepresentation_Keyed",
			"UnionRepresentation_Envelope",
			"UnionRepresentation_Inline",
			"UnionRepresentation_StringPrefix",
			"UnionRepresentation_BytesPrefix",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"kinded":       "UnionRepresentation_Kinded",
			"keyed":        "UnionRepresentation_Keyed",
			"envelope":     "UnionRepresentation_Envelope",
			"inline":       "UnionRepresentation_Inline",
			"stringprefix": "UnionRepresentation_StringPrefix",
			"byteprefix":   "UnionRepresentation_BytesPrefix",
		}),
	))
	ts.Accumulate(schema.SpawnMap("UnionRepresentation_Kinded",
		"RepresentationKind", "UnionMember", false,
	))
	ts.Accumulate(schema.SpawnMap("UnionRepresentation_Keyed",
		"String", "UnionMember", false,
	))
	ts.Accumulate(schema.SpawnMap("Map__String__UnionMember",
		"TypeName", "TypeDefn", false,
	))
	ts.Accumulate(schema.SpawnStruct("UnionRepresentation_Envelope",
		[]schema.StructField{
			schema.SpawnStructField("discriminantKey", "String", false, false),
			schema.SpawnStructField("contentKey", "String", false, false),
			schema.SpawnStructField("discriminantTable", "Map__String__UnionMember", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("UnionRepresentation_Inline",
		[]schema.StructField{
			schema.SpawnStructField("discriminantKey", "String", false, false),
			schema.SpawnStructField("discriminantTable", "Map__String__TypeName", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("UnionRepresentation_StringPrefix",
		[]schema.StructField{
			schema.SpawnStructField("prefixes", "Map__String__TypeName", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("UnionRepresentation_BytesPrefix",
		[]schema.StructField{
			schema.SpawnStructField("prefixes", "Map__HexString__TypeName", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnMap("Map__HexString__TypeName",
		"String", "TypeName", false,
	))
	ts.Accumulate(schema.SpawnString("HexString"))
	ts.Accumulate(schema.SpawnMap("Map__String__TypeName",
		"String", "TypeName", false,
	))
	ts.Accumulate(schema.SpawnMap("Map__TypeName__Int",
		"String", "Int", false,
	))
	ts.Accumulate(schema.SpawnString("RepresentationKind")) // todo: RepresentationKind is supposed to be an enum, but we're puting it to a string atm.
	ts.Accumulate(schema.SpawnStruct("TypeDefnStruct",
		[]schema.StructField{
			schema.SpawnStructField("fields", "Map__FieldName__StructField", false, false), // todo: dodging inline defn's again.
			schema.SpawnStructField("representation", "StructRepresentation", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnMap("Map__FieldName__StructField",
		"FieldName", "StructField", false,
	))
	ts.Accumulate(schema.SpawnString("FieldName"))
	ts.Accumulate(schema.SpawnStruct("StructField",
		[]schema.StructField{
			schema.SpawnStructField("type", "TypeNameOrInlineDefn", false, false),
			schema.SpawnStructField("optional", "Bool", true, false), // todo: wants to use the "implicit" feature, but not supported yet
			schema.SpawnStructField("nullable", "Bool", true, false), // todo: wants to use the "implicit" feature, but not supported yet
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnUnion("StructRepresentation",
		[]schema.TypeName{
			"StructRepresentation_Map",
			"StructRepresentation_Tuple",
			"StructRepresentation_Stringpairs",
			"StructRepresentation_Stringjoin",
			"StructRepresentation_Listpairs",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"map":         "StructRepresentation_Map",
			"tuple":       "StructRepresentation_Tuple",
			"stringpairs": "StructRepresentation_Stringpairs",
			"stringjoin":  "StructRepresentation_Stringjoin",
			"listpairs":   "StructRepresentation_Listpairs",
		}),
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Map",
		[]schema.StructField{
			schema.SpawnStructField("fields", "Map__FieldName__StructRepresentation_Map_FieldDetails", true, false), // todo: dodging inline defn's again.
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnMap("Map__FieldName__StructRepresentation_Map_FieldDetails",
		"FieldName", "StructRepresentation_Map_FieldDetails", false,
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Map_FieldDetails",
		[]schema.StructField{
			schema.SpawnStructField("rename", "String", true, false),
			schema.SpawnStructField("implicit", "AnyScalar", true, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Tuple",
		[]schema.StructField{
			schema.SpawnStructField("fieldOrder", "List__FieldName", true, false), // todo: dodging inline defn's again.
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnList("List__FieldName",
		"FieldName", false,
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Stringpairs",
		[]schema.StructField{
			schema.SpawnStructField("innerDelim", "String", false, false),
			schema.SpawnStructField("entryDelim", "String", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Stringjoin",
		[]schema.StructField{
			schema.SpawnStructField("join", "String", false, false),               // review: "delim" would seem more consistent with others -- but this is currently what the schema-schema says.
			schema.SpawnStructField("fieldOrder", "List__FieldName", true, false), // todo: dodging inline defn's again.
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("StructRepresentation_Listpairs",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnEnum",
		[]schema.StructField{
			schema.SpawnStructField("members", "List__EnumMember", false, false),
			schema.SpawnStructField("representation", "EnumRepresentation", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("Unit", // todo: we should formalize the introdution of unit as first class type kind.
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnList("List__EnumMember",
		"EnumMember", false,
	))
	ts.Accumulate(schema.SpawnString("EnumMember"))
	ts.Accumulate(schema.SpawnUnion("EnumRepresentation",
		[]schema.TypeName{
			"EnumRepresentation_String",
			"EnumRepresentation_Int",
		},
		schema.SpawnUnionRepresentationKeyed(map[string]schema.TypeName{
			"string": "EnumRepresentation_String",
			"int":    "EnumRepresentation_Int",
		}),
	))
	ts.Accumulate(schema.SpawnMap("EnumRepresentation_String",
		"EnumMember", "String", false,
	))
	ts.Accumulate(schema.SpawnMap("EnumRepresentation_Int",
		"EnumMember", "Int", false,
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnUnit",
		[]schema.StructField{
			schema.SpawnStructField("representation", "UnitRepresentation", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnString("UnitRepresentation")) // TODO: enum
	ts.Accumulate(schema.SpawnStruct("TypeDefnAny",
		[]schema.StructField{},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnStruct("TypeDefnCopy",
		[]schema.StructField{
			schema.SpawnStructField("fromType", "TypeName", false, false),
		},
		schema.StructRepresentation_Map{},
	))
	ts.Accumulate(schema.SpawnUnion("AnyScalar",
		[]schema.TypeName{
			"Bool",
			"String",
			"Bytes",
			"Int",
			"Float",
		},
		schema.SpawnUnionRepresentationKinded(map[datamodel.Kind]schema.TypeName{
			datamodel.Kind_Bool:   "Bool",
			datamodel.Kind_String: "String",
			datamodel.Kind_Bytes:  "Bytes",
			datamodel.Kind_Int:    "Int",
			datamodel.Kind_Float:  "Float",
		}),
	))

	if errs := ts.ValidateGraph(); errs != nil {
		for _, err := range errs {
			fmt.Printf("- %s\n", err)
		}
		panic("not happening")
	}

	schemaTypeSystem = ts

	Type.Schema = bindnode.Prototype(
		(*Schema)(nil),
		schemaTypeSystem.TypeByName("Schema"),
	)
}
