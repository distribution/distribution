package schemadmt

type Schema struct {
	Types Map__TypeName__TypeDefn
}
type Map__TypeName__TypeDefn struct {
	Keys   []string
	Values map[string]TypeDefn
}
type TypeDefn struct {
	TypeDefnBool   *TypeDefnBool
	TypeDefnString *TypeDefnString
	TypeDefnBytes  *TypeDefnBytes
	TypeDefnInt    *TypeDefnInt
	TypeDefnFloat  *TypeDefnFloat
	TypeDefnMap    *TypeDefnMap
	TypeDefnList   *TypeDefnList
	TypeDefnLink   *TypeDefnLink
	TypeDefnUnion  *TypeDefnUnion
	TypeDefnStruct *TypeDefnStruct
	TypeDefnEnum   *TypeDefnEnum
	TypeDefnUnit   *TypeDefnUnit
	TypeDefnAny    *TypeDefnAny
	TypeDefnCopy   *TypeDefnCopy
}
type TypeNameOrInlineDefn struct {
	TypeName   *string
	InlineDefn *InlineDefn
}
type InlineDefn struct {
	TypeDefnMap  *TypeDefnMap
	TypeDefnList *TypeDefnList
	TypeDefnLink *TypeDefnLink
}
type TypeDefnBool struct {
}
type TypeDefnString struct {
}
type TypeDefnBytes struct {
}
type TypeDefnInt struct {
}
type TypeDefnFloat struct {
}
type TypeDefnMap struct {
	KeyType        string
	ValueType      TypeNameOrInlineDefn
	ValueNullable  *bool
	Representation *MapRepresentation
}
type MapRepresentation struct {
	MapRepresentation_Map         *MapRepresentation_Map
	MapRepresentation_Stringpairs *MapRepresentation_Stringpairs
	MapRepresentation_Listpairs   *MapRepresentation_Listpairs
}
type MapRepresentation_Map struct {
}
type MapRepresentation_Stringpairs struct {
	InnerDelim string
	EntryDelim string
}
type MapRepresentation_Listpairs struct {
}
type TypeDefnList struct {
	ValueType      TypeNameOrInlineDefn
	ValueNullable  *bool
	Representation *ListRepresentation
}
type ListRepresentation struct {
	ListRepresentation_List *ListRepresentation_List
}
type ListRepresentation_List struct {
}
type TypeDefnUnion struct {
	Members        List__UnionMember
	Representation UnionRepresentation
}
type List__UnionMember []UnionMember
type UnionMember struct {
	TypeName              *string
	UnionMemberInlineDefn *UnionMemberInlineDefn
}
type UnionMemberInlineDefn struct {
	TypeDefnLink *TypeDefnLink
}
type List__TypeName []string
type TypeDefnLink struct {
	ExpectedType *string
}
type UnionRepresentation struct {
	UnionRepresentation_Kinded       *UnionRepresentation_Kinded
	UnionRepresentation_Keyed        *UnionRepresentation_Keyed
	UnionRepresentation_Envelope     *UnionRepresentation_Envelope
	UnionRepresentation_Inline       *UnionRepresentation_Inline
	UnionRepresentation_StringPrefix *UnionRepresentation_StringPrefix
	UnionRepresentation_BytesPrefix  *UnionRepresentation_BytesPrefix
}
type UnionRepresentation_Kinded struct {
	Keys   []string
	Values map[string]UnionMember
}
type UnionRepresentation_Keyed struct {
	Keys   []string
	Values map[string]UnionMember
}
type Map__String__UnionMember struct {
	Keys   []string
	Values map[string]TypeDefn
}
type UnionRepresentation_Envelope struct {
	DiscriminantKey   string
	ContentKey        string
	DiscriminantTable Map__String__UnionMember
}
type UnionRepresentation_Inline struct {
	DiscriminantKey   string
	DiscriminantTable Map__String__TypeName
}
type UnionRepresentation_StringPrefix struct {
	Prefixes Map__String__TypeName
}
type UnionRepresentation_BytesPrefix struct {
	Prefixes Map__HexString__TypeName
}
type Map__HexString__TypeName struct {
	Keys   []string
	Values map[string]string
}
type Map__String__TypeName struct {
	Keys   []string
	Values map[string]string
}
type Map__TypeName__Int struct {
	Keys   []string
	Values map[string]int
}
type TypeDefnStruct struct {
	Fields         Map__FieldName__StructField
	Representation StructRepresentation
}
type Map__FieldName__StructField struct {
	Keys   []string
	Values map[string]StructField
}
type StructField struct {
	Type     TypeNameOrInlineDefn
	Optional *bool
	Nullable *bool
}
type StructRepresentation struct {
	StructRepresentation_Map         *StructRepresentation_Map
	StructRepresentation_Tuple       *StructRepresentation_Tuple
	StructRepresentation_Stringpairs *StructRepresentation_Stringpairs
	StructRepresentation_Stringjoin  *StructRepresentation_Stringjoin
	StructRepresentation_Listpairs   *StructRepresentation_Listpairs
}
type StructRepresentation_Map struct {
	Fields *Map__FieldName__StructRepresentation_Map_FieldDetails
}
type Map__FieldName__StructRepresentation_Map_FieldDetails struct {
	Keys   []string
	Values map[string]StructRepresentation_Map_FieldDetails
}
type StructRepresentation_Map_FieldDetails struct {
	Rename   *string
	Implicit *AnyScalar
}
type StructRepresentation_Tuple struct {
	FieldOrder *List__FieldName
}
type List__FieldName []string
type StructRepresentation_Stringpairs struct {
	InnerDelim string
	EntryDelim string
}
type StructRepresentation_Stringjoin struct {
	Join       string
	FieldOrder *List__FieldName
}
type StructRepresentation_Listpairs struct {
}
type TypeDefnEnum struct {
	Members        List__EnumMember
	Representation EnumRepresentation
}
type Unit struct {
}
type List__EnumMember []string
type EnumRepresentation struct {
	EnumRepresentation_String *EnumRepresentation_String
	EnumRepresentation_Int    *EnumRepresentation_Int
}
type EnumRepresentation_String struct {
	Keys   []string
	Values map[string]string
}
type EnumRepresentation_Int struct {
	Keys   []string
	Values map[string]int
}
type TypeDefnUnit struct {
	Representation string
}
type TypeDefnAny struct {
}
type TypeDefnCopy struct {
	FromType string
}
type AnyScalar struct {
	Bool   *bool
	String *string
	Bytes  *[]uint8
	Int    *int
	Float  *float64
}
