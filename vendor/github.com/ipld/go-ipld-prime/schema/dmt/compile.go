package schemadmt

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/schema"
)

// Compile transforms a schema in DMT form into a TypeSystem.
//
// Note that this API is EXPERIMENTAL and will likely change.
// It is also unfinished and buggy.
func Compile(ts *schema.TypeSystem, node *Schema) error {
	// Prelude; probably belongs elsewhere.
	{
		ts.Accumulate(schema.SpawnBool("Bool"))
		ts.Accumulate(schema.SpawnInt("Int"))
		ts.Accumulate(schema.SpawnFloat("Float"))
		ts.Accumulate(schema.SpawnString("String"))
		ts.Accumulate(schema.SpawnBytes("Bytes"))

		ts.Accumulate(schema.SpawnAny("Any"))

		ts.Accumulate(schema.SpawnMap("Map", "String", "Any", false))
		ts.Accumulate(schema.SpawnList("List", "Any", false))

		// Should be &Any, really.
		ts.Accumulate(schema.SpawnLink("Link"))

		// TODO: schema package lacks support?
		// ts.Accumulate(schema.SpawnUnit("Null", NullRepr))
	}

	for _, name := range node.Types.Keys {
		defn := node.Types.Values[name]

		// TODO: once ./schema supports anonymous/inline types, remove the ts argument.
		typ, err := spawnType(ts, name, defn)
		if err != nil {
			return err
		}
		ts.Accumulate(typ)
	}

	if errs := ts.ValidateGraph(); errs != nil {
		// Return the first error.
		for _, err := range errs {
			return err
		}
	}
	return nil
}

// Note that the parser and compiler support defaults. We're lacking support in bindnode.
func todoFromImplicitlyFalseBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func todoAnonTypeName(nameOrDefn TypeNameOrInlineDefn) string {
	if nameOrDefn.TypeName != nil {
		return *nameOrDefn.TypeName
	}
	defn := *nameOrDefn.InlineDefn
	switch {
	case defn.TypeDefnMap != nil:
		defn := defn.TypeDefnMap
		return fmt.Sprintf("Map__%s__%s", defn.KeyType, todoAnonTypeName(defn.ValueType))
	case defn.TypeDefnList != nil:
		defn := defn.TypeDefnList
		return fmt.Sprintf("List__%s", todoAnonTypeName(defn.ValueType))
	default:
		panic(fmt.Errorf("%#v", defn))
	}
}

func parseKind(s string) datamodel.Kind {
	switch s {
	case "map":
		return datamodel.Kind_Map
	case "list":
		return datamodel.Kind_List
	case "null":
		return datamodel.Kind_Null
	case "bool":
		return datamodel.Kind_Bool
	case "int":
		return datamodel.Kind_Int
	case "float":
		return datamodel.Kind_Float
	case "string":
		return datamodel.Kind_String
	case "bytes":
		return datamodel.Kind_Bytes
	case "link":
		return datamodel.Kind_Link
	default:
		return datamodel.Kind_Invalid
	}
}

func spawnType(ts *schema.TypeSystem, name schema.TypeName, defn TypeDefn) (schema.Type, error) {
	switch {
	// Scalar types without parameters.
	case defn.TypeDefnBool != nil:
		return schema.SpawnBool(name), nil
	case defn.TypeDefnString != nil:
		return schema.SpawnString(name), nil
	case defn.TypeDefnBytes != nil:
		return schema.SpawnBytes(name), nil
	case defn.TypeDefnInt != nil:
		return schema.SpawnInt(name), nil
	case defn.TypeDefnFloat != nil:
		return schema.SpawnFloat(name), nil

	case defn.TypeDefnList != nil:
		typ := defn.TypeDefnList
		if typ.ValueType.InlineDefn != nil {
			return nil, fmt.Errorf("TODO: support anonymous types in schema package")
		}
		switch {
		case typ.Representation == nil ||
			typ.Representation.ListRepresentation_List != nil:
			// default behavior
		default:
			return nil, fmt.Errorf("TODO: support other map repr in schema package")
		}
		return schema.SpawnList(name,
			*typ.ValueType.TypeName,
			todoFromImplicitlyFalseBool(typ.ValueNullable),
		), nil
	case defn.TypeDefnMap != nil:
		typ := defn.TypeDefnMap
		if typ.ValueType.InlineDefn != nil {
			return nil, fmt.Errorf("TODO: support anonymous types in schema package")
		}
		switch {
		case typ.Representation == nil ||
			typ.Representation.MapRepresentation_Map != nil:
			// default behavior
		default:
			return nil, fmt.Errorf("TODO: support other map repr in schema package")
		}
		return schema.SpawnMap(name,
			typ.KeyType,
			*typ.ValueType.TypeName,
			todoFromImplicitlyFalseBool(typ.ValueNullable),
		), nil
	case defn.TypeDefnStruct != nil:
		typ := defn.TypeDefnStruct
		var fields []schema.StructField
		for _, fname := range typ.Fields.Keys {
			field := typ.Fields.Values[fname]
			tname := ""
			if field.Type.TypeName != nil {
				tname = *field.Type.TypeName
			} else if tname = todoAnonTypeName(field.Type); ts.TypeByName(tname) == nil {
				// Note that TypeDefn and InlineDefn aren't the same enum.
				anonDefn := TypeDefn{
					TypeDefnMap:  field.Type.InlineDefn.TypeDefnMap,
					TypeDefnList: field.Type.InlineDefn.TypeDefnList,
					TypeDefnLink: field.Type.InlineDefn.TypeDefnLink,
				}
				anonType, err := spawnType(ts, tname, anonDefn)
				if err != nil {
					return nil, err
				}
				ts.Accumulate(anonType)
			}
			fields = append(fields, schema.SpawnStructField(fname,
				tname,
				todoFromImplicitlyFalseBool(field.Optional),
				todoFromImplicitlyFalseBool(field.Nullable),
			))
		}
		var repr schema.StructRepresentation
		switch {
		case typ.Representation.StructRepresentation_Map != nil:
			rp := typ.Representation.StructRepresentation_Map
			if rp.Fields == nil {
				repr = schema.SpawnStructRepresentationMap2(nil, nil)
				break
			}
			renames := make(map[string]string, len(rp.Fields.Keys))
			implicits := make(map[string]schema.ImplicitValue, len(rp.Fields.Keys))
			for _, name := range rp.Fields.Keys {
				details := rp.Fields.Values[name]
				if details.Rename != nil {
					renames[name] = *details.Rename
				}
				if imp := details.Implicit; imp != nil {
					var sumVal schema.ImplicitValue
					switch {
					case imp.Bool != nil:
						sumVal = schema.ImplicitValue_Bool(*imp.Bool)
					case imp.String != nil:
						sumVal = schema.ImplicitValue_String(*imp.String)
					case imp.Int != nil:
						sumVal = schema.ImplicitValue_Int(*imp.Int)
					default:
						panic("TODO: implicit value kind")
					}
					implicits[name] = sumVal
				}

			}
			repr = schema.SpawnStructRepresentationMap2(renames, implicits)
		case typ.Representation.StructRepresentation_Tuple != nil:
			rp := typ.Representation.StructRepresentation_Tuple
			if rp.FieldOrder == nil {
				repr = schema.SpawnStructRepresentationTuple()
				break
			}
			return nil, fmt.Errorf("TODO: support for tuples with field orders in the schema package")
		default:
			return nil, fmt.Errorf("TODO: support other struct repr in schema package")
		}
		return schema.SpawnStruct(name,
			fields,
			repr,
		), nil
	case defn.TypeDefnUnion != nil:
		typ := defn.TypeDefnUnion
		var members []schema.TypeName
		for _, member := range typ.Members {
			if member.TypeName != nil {
				members = append(members, *member.TypeName)
			} else {
				panic("TODO: inline union members")
			}
		}
		var repr schema.UnionRepresentation
		switch {
		case typ.Representation.UnionRepresentation_Kinded != nil:
			rp := typ.Representation.UnionRepresentation_Kinded
			table := make(map[datamodel.Kind]schema.TypeName, len(rp.Keys))
			for _, kindStr := range rp.Keys {
				kind := parseKind(kindStr)
				member := rp.Values[kindStr]
				switch {
				case member.TypeName != nil:
					table[kind] = *member.TypeName
				case member.UnionMemberInlineDefn != nil:
					panic("TODO: inline defn support")
				}
			}
			repr = schema.SpawnUnionRepresentationKinded(table)
		case typ.Representation.UnionRepresentation_Keyed != nil:
			rp := typ.Representation.UnionRepresentation_Keyed
			table := make(map[string]schema.TypeName, len(rp.Keys))
			for _, key := range rp.Keys {
				member := rp.Values[key]
				switch {
				case member.TypeName != nil:
					table[key] = *member.TypeName
				case member.UnionMemberInlineDefn != nil:
					panic("TODO: inline defn support")
				}
			}
			repr = schema.SpawnUnionRepresentationKeyed(table)
		default:
			return nil, fmt.Errorf("TODO: support other union repr in schema package")
		}
		return schema.SpawnUnion(name,
			members,
			repr,
		), nil
	case defn.TypeDefnEnum != nil:
		typ := defn.TypeDefnEnum
		var repr schema.EnumRepresentation
		switch {
		case typ.Representation.EnumRepresentation_String != nil:
			rp := typ.Representation.EnumRepresentation_String
			repr = schema.EnumRepresentation_String(rp.Values)
		case typ.Representation.EnumRepresentation_Int != nil:
			rp := typ.Representation.EnumRepresentation_Int
			repr = schema.EnumRepresentation_Int(rp.Values)
		default:
			return nil, fmt.Errorf("TODO: support other enum repr in schema package")
		}
		return schema.SpawnEnum(name,
			typ.Members,
			repr,
		), nil
	case defn.TypeDefnLink != nil:
		typ := defn.TypeDefnLink
		if typ.ExpectedType == nil {
			return schema.SpawnLink(name), nil
		}
		return schema.SpawnLinkReference(name, *typ.ExpectedType), nil
	case defn.TypeDefnAny != nil:
		return schema.SpawnAny(name), nil
	default:
		panic(fmt.Errorf("%#v", defn))
	}
}
