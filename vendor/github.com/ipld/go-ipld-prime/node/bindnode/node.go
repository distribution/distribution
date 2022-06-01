package bindnode

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/ipld/go-ipld-prime/schema"
)

// Assert that we implement all the interfaces as expected.
// Grouped by the interfaces to implement, roughly.
var (
	_ datamodel.NodePrototype = (*_prototype)(nil)
	_ schema.TypedPrototype   = (*_prototype)(nil)
	_ datamodel.NodePrototype = (*_prototypeRepr)(nil)

	_ datamodel.Node   = (*_node)(nil)
	_ schema.TypedNode = (*_node)(nil)
	_ datamodel.Node   = (*_nodeRepr)(nil)

	_ datamodel.NodeBuilder   = (*_builder)(nil)
	_ datamodel.NodeBuilder   = (*_builderRepr)(nil)
	_ datamodel.NodeAssembler = (*_assembler)(nil)
	_ datamodel.NodeAssembler = (*_assemblerRepr)(nil)

	_ datamodel.MapAssembler = (*_structAssembler)(nil)
	_ datamodel.MapAssembler = (*_structAssemblerRepr)(nil)
	_ datamodel.MapIterator  = (*_structIterator)(nil)
	_ datamodel.MapIterator  = (*_structIteratorRepr)(nil)

	_ datamodel.ListAssembler = (*_listAssembler)(nil)
	_ datamodel.ListAssembler = (*_listAssemblerRepr)(nil)
	_ datamodel.ListIterator  = (*_listIterator)(nil)
	_ datamodel.ListIterator  = (*_tupleIteratorRepr)(nil)

	_ datamodel.MapAssembler = (*_unionAssembler)(nil)
	_ datamodel.MapAssembler = (*_unionAssemblerRepr)(nil)
	_ datamodel.MapIterator  = (*_unionIterator)(nil)
	_ datamodel.MapIterator  = (*_unionIteratorRepr)(nil)
)

type _prototype struct {
	schemaType schema.Type
	goType     reflect.Type // non-pointer
}

func (w *_prototype) NewBuilder() datamodel.NodeBuilder {
	return &_builder{_assembler{
		schemaType: w.schemaType,
		val:        reflect.New(w.goType).Elem(),
	}}
}

func (w *_prototype) Type() schema.Type {
	return w.schemaType
}

func (w *_prototype) Representation() datamodel.NodePrototype {
	return (*_prototypeRepr)(w)
}

type _node struct {
	schemaType schema.Type

	val reflect.Value // non-pointer
}

// TODO: only expose TypedNode methods if the schema was explicit.
// type _typedNode struct {
// 	_node
// }

func (w *_node) Type() schema.Type {
	return w.schemaType
}

func (w *_node) Representation() datamodel.Node {
	return (*_nodeRepr)(w)
}

func (w *_node) Kind() datamodel.Kind {
	return actualKind(w.schemaType)
}

func compatibleKind(schemaType schema.Type, kind datamodel.Kind) error {
	switch sch := schemaType.(type) {
	case *schema.TypeAny:
		return nil
	default:
		actual := actualKind(sch)
		if actual == kind {
			return nil
		}
		methodName := ""
		if pc, _, _, ok := runtime.Caller(1); ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				methodName = fn.Name()
				// Go from "pkg/path.Type.Method" to just "Method".
				methodName = methodName[strings.LastIndexByte(methodName, '.')+1:]
			}
		}

		return datamodel.ErrWrongKind{
			TypeName:        schemaType.Name(),
			MethodName:      methodName,
			AppropriateKind: datamodel.KindSet{kind},
			ActualKind:      actual,
		}
	}
}

func actualKind(schemaType schema.Type) datamodel.Kind {
	return schemaType.TypeKind().ActsLike()
}

func nonPtrVal(val reflect.Value) reflect.Value {
	// TODO: support **T as well as *T?
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			// TODO: error in this case?
			return reflect.Value{}
		}
		val = val.Elem()
	}
	return val
}

func (w *_node) LookupByString(key string) (datamodel.Node, error) {
	switch typ := w.schemaType.(type) {
	case *schema.TypeStruct:
		field := typ.Field(key)
		if field == nil {
			return nil, schema.ErrInvalidKey{
				TypeName: typ.Name(),
				Key:      basicnode.NewString(key),
			}
		}
		fval := nonPtrVal(w.val).FieldByName(fieldNameFromSchema(key))
		if !fval.IsValid() {
			return nil, fmt.Errorf("bindnode TODO: go-schema mismatch")
		}
		if field.IsOptional() {
			if fval.IsNil() {
				return datamodel.Absent, nil
			}
			fval = fval.Elem()
		}
		if field.IsNullable() {
			if fval.IsNil() {
				return datamodel.Null, nil
			}
			fval = fval.Elem()
		}
		if _, ok := field.Type().(*schema.TypeAny); ok {
			return nonPtrVal(fval).Interface().(datamodel.Node), nil
		}
		node := &_node{
			schemaType: field.Type(),
			val:        fval,
		}
		return node, nil
	case *schema.TypeMap:
		var kval reflect.Value
		valuesVal := nonPtrVal(w.val).FieldByName("Values")
		switch ktyp := typ.KeyType().(type) {
		case *schema.TypeString:
			kval = reflect.ValueOf(key)
		default:
			asm := &_assembler{
				schemaType: ktyp,
				val:        reflect.New(valuesVal.Type().Key()).Elem(),
			}
			if err := (*_assemblerRepr)(asm).AssignString(key); err != nil {
				return nil, err
			}
			kval = asm.val
		}
		fval := valuesVal.MapIndex(kval)
		if !fval.IsValid() { // not found
			return nil, datamodel.ErrNotExists{Segment: datamodel.PathSegmentOfString(key)}
		}
		// TODO: Error/panic if fval.IsNil() && !typ.ValueIsNullable()?
		// Otherwise we could have two non-equal Go values (nil map,
		// non-nil-but-empty map) which represent the exact same IPLD
		// node when the field is not nullable.
		if typ.ValueIsNullable() {
			if fval.IsNil() {
				return datamodel.Null, nil
			}
			fval = fval.Elem()
		}
		if _, ok := typ.ValueType().(*schema.TypeAny); ok {
			return nonPtrVal(fval).Interface().(datamodel.Node), nil
		}
		node := &_node{
			schemaType: typ.ValueType(),
			val:        fval,
		}
		return node, nil
	case *schema.TypeUnion:
		var idx int
		var mtyp schema.Type
		for i, member := range typ.Members() {
			if member.Name() == key {
				idx = i
				mtyp = member
				break
			}
		}
		if mtyp == nil { // not found
			return nil, datamodel.ErrNotExists{Segment: datamodel.PathSegmentOfString(key)}
		}
		// TODO: we could look up the right Go field straight away via idx.
		haveIdx, mval := unionMember(nonPtrVal(w.val))
		if haveIdx != idx { // mismatching type
			return nil, datamodel.ErrNotExists{Segment: datamodel.PathSegmentOfString(key)}
		}
		node := &_node{
			schemaType: mtyp,
			val:        mval,
		}
		return node, nil
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "LookupByString",
		AppropriateKind: datamodel.KindSet_JustMap,
		ActualKind:      w.Kind(),
	}
}

var invalidValue reflect.Value

func unionMember(val reflect.Value) (int, reflect.Value) {
	// The first non-nil field is a match.
	for i := 0; i < val.NumField(); i++ {
		elemVal := val.Field(i)
		if elemVal.Kind() != reflect.Ptr {
			panic("bindnode bug: found unexpected non-pointer in a union field")
		}
		if elemVal.IsNil() {
			continue
		}
		return i, elemVal.Elem()
	}
	return -1, invalidValue
}

func unionSetMember(val reflect.Value, memberIdx int, memberPtr reflect.Value) {
	// Reset the entire union struct to zero, to clear any non-nil pointers.
	val.Set(reflect.Zero(val.Type()))

	// Set the index pointer to the given value.
	val.Field(memberIdx).Set(memberPtr)
}

func (w *_node) LookupByIndex(idx int64) (datamodel.Node, error) {
	switch typ := w.schemaType.(type) {
	case *schema.TypeList:
		val := nonPtrVal(w.val)
		if idx < 0 || int(idx) >= val.Len() {
			return nil, datamodel.ErrNotExists{Segment: datamodel.PathSegmentOfInt(idx)}
		}
		val = val.Index(int(idx))
		if typ.ValueIsNullable() {
			if val.IsNil() {
				return datamodel.Null, nil
			}
			val = val.Elem()
		}
		if _, ok := typ.ValueType().(*schema.TypeAny); ok {
			return nonPtrVal(val).Interface().(datamodel.Node), nil
		}
		return &_node{schemaType: typ.ValueType(), val: val}, nil
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "LookupByIndex",
		AppropriateKind: datamodel.KindSet_JustList,
		ActualKind:      w.Kind(),
	}
}

func (w *_node) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	switch w.Kind() {
	case datamodel.Kind_Map:
		return w.LookupByString(seg.String())
	case datamodel.Kind_List:
		idx, err := seg.Index()
		if err != nil {
			return nil, err
		}
		return w.LookupByIndex(idx)
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "LookupBySegment",
		AppropriateKind: datamodel.KindSet_Recursive,
		ActualKind:      w.Kind(),
	}
}

func (w *_node) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	switch w.Kind() {
	case datamodel.Kind_Map:
		s, err := key.AsString()
		if err != nil {
			return nil, err
		}
		return w.LookupByString(s)
	case datamodel.Kind_List:
		i, err := key.AsInt()
		if err != nil {
			return nil, err
		}
		return w.LookupByIndex(i)
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "LookupByNode",
		AppropriateKind: datamodel.KindSet_Recursive,
		ActualKind:      w.Kind(),
	}
}

func (w *_node) MapIterator() datamodel.MapIterator {
	val := nonPtrVal(w.val)
	switch typ := w.schemaType.(type) {
	case *schema.TypeStruct:
		return &_structIterator{
			schemaType: typ,
			fields:     typ.Fields(),
			val:        val,
		}
	case *schema.TypeUnion:
		return &_unionIterator{
			schemaType: typ,
			members:    typ.Members(),
			val:        val,
		}
	case *schema.TypeMap:
		return &_mapIterator{
			schemaType: typ,
			keysVal:    val.FieldByName("Keys"),
			valuesVal:  val.FieldByName("Values"),
		}
	}
	return nil
}

func (w *_node) ListIterator() datamodel.ListIterator {
	val := nonPtrVal(w.val)
	switch typ := w.schemaType.(type) {
	case *schema.TypeList:
		return &_listIterator{schemaType: typ, val: val}
	}
	return nil
}

func (w *_node) Length() int64 {
	val := nonPtrVal(w.val)
	switch w.Kind() {
	case datamodel.Kind_Map:
		switch typ := w.schemaType.(type) {
		case *schema.TypeStruct:
			return int64(len(typ.Fields()))
		case *schema.TypeUnion:
			return 1
		}
		return int64(val.FieldByName("Keys").Len())
	case datamodel.Kind_List:
		return int64(val.Len())
	}
	return -1
}

// TODO: better story around pointers and absent/null

func (w *_node) IsAbsent() bool {
	return false
}

func (w *_node) IsNull() bool {
	return false
}

func (w *_node) AsBool() (bool, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Bool); err != nil {
		return false, err
	}
	return nonPtrVal(w.val).Bool(), nil
}

func (w *_node) AsInt() (int64, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Int); err != nil {
		return 0, err
	}
	val := nonPtrVal(w.val)
	if kindUint[val.Kind()] {
		// TODO: check for overflow
		return int64(val.Uint()), nil
	}
	return val.Int(), nil
}

func (w *_node) AsFloat() (float64, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Float); err != nil {
		return 0, err
	}
	return nonPtrVal(w.val).Float(), nil
}

func (w *_node) AsString() (string, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_String); err != nil {
		return "", err
	}
	return nonPtrVal(w.val).String(), nil
}

func (w *_node) AsBytes() ([]byte, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Bytes); err != nil {
		return nil, err
	}
	return nonPtrVal(w.val).Bytes(), nil
}

func (w *_node) AsLink() (datamodel.Link, error) {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Link); err != nil {
		return nil, err
	}
	switch val := nonPtrVal(w.val).Interface().(type) {
	case datamodel.Link:
		return val, nil
	case cid.Cid:
		return cidlink.Link{Cid: val}, nil
	default:
		return nil, fmt.Errorf("bindnode: unexpected link type %T", val)
	}
}

func (w *_node) Prototype() datamodel.NodePrototype {
	return &_prototype{schemaType: w.schemaType, goType: w.val.Type()}
}

type _builder struct {
	_assembler
}

func (w *_builder) Build() datamodel.Node {
	// TODO: should we panic if no Assign call was made, just like codegen?
	return &_node{schemaType: w.schemaType, val: w.val}
}

func (w *_builder) Reset() {
	panic("bindnode TODO: Reset")
}

type _assembler struct {
	schemaType schema.Type
	val        reflect.Value // non-pointer
	finish     func() error

	// kinded   bool // true if val is interface{} for a kinded union
	nullable bool // true if field or map value is nullable
}

func (w *_assembler) createNonPtrVal() reflect.Value {
	val := w.val
	// TODO: support **T as well as *T?
	if val.Kind() == reflect.Ptr {
		// TODO: Sometimes we call createNonPtrVal before an assignment actually
		// happens. Does that matter?
		// If it matters and we only want to modify the destination value on
		// success, then we should make use of the "finish" func.
		val.Set(reflect.New(val.Type().Elem()))
		val = val.Elem()
	}
	return val
}

func (w *_assembler) Representation() datamodel.NodeAssembler {
	return (*_assemblerRepr)(w)
}

type basicMapAssembler struct {
	datamodel.MapAssembler

	builder datamodel.NodeBuilder
	parent  *_assembler
}

func (w *basicMapAssembler) Finish() error {
	if err := w.MapAssembler.Finish(); err != nil {
		return err
	}
	basicNode := w.builder.Build()
	w.parent.createNonPtrVal().Set(reflect.ValueOf(basicNode))
	if w.parent.finish != nil {
		if err := w.parent.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	switch typ := w.schemaType.(type) {
	case *schema.TypeAny:
		basicBuilder := basicnode.Prototype.Any.NewBuilder()
		mapAsm, err := basicBuilder.BeginMap(sizeHint)
		if err != nil {
			return nil, err
		}
		return &basicMapAssembler{MapAssembler: mapAsm, builder: basicBuilder, parent: w}, nil
	case *schema.TypeStruct:
		val := w.createNonPtrVal()
		doneFields := make([]bool, val.NumField())
		return &_structAssembler{
			schemaType: typ,
			val:        val,
			doneFields: doneFields,
			finish:     w.finish,
		}, nil
	case *schema.TypeMap:
		val := w.createNonPtrVal()
		keysVal := val.FieldByName("Keys")
		valuesVal := val.FieldByName("Values")
		if valuesVal.IsNil() {
			valuesVal.Set(reflect.MakeMap(valuesVal.Type()))
		}
		return &_mapAssembler{
			schemaType: typ,
			keysVal:    keysVal,
			valuesVal:  valuesVal,
			finish:     w.finish,
		}, nil
	case *schema.TypeUnion:
		val := w.createNonPtrVal()
		return &_unionAssembler{
			schemaType: typ,
			val:        val,
			finish:     w.finish,
		}, nil
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "BeginMap",
		AppropriateKind: datamodel.KindSet_JustMap,
		ActualKind:      actualKind(w.schemaType),
	}
}

type basicListAssembler struct {
	datamodel.ListAssembler

	builder datamodel.NodeBuilder
	parent  *_assembler
}

func (w *basicListAssembler) Finish() error {
	if err := w.ListAssembler.Finish(); err != nil {
		return err
	}
	basicNode := w.builder.Build()
	w.parent.createNonPtrVal().Set(reflect.ValueOf(basicNode))
	if w.parent.finish != nil {
		if err := w.parent.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	switch typ := w.schemaType.(type) {
	case *schema.TypeAny:
		basicBuilder := basicnode.Prototype.Any.NewBuilder()
		listAsm, err := basicBuilder.BeginList(sizeHint)
		if err != nil {
			return nil, err
		}
		return &basicListAssembler{ListAssembler: listAsm, builder: basicBuilder, parent: w}, nil
	case *schema.TypeList:
		val := w.createNonPtrVal()
		return &_listAssembler{
			schemaType: typ,
			val:        val,
			finish:     w.finish,
		}, nil
	}
	return nil, datamodel.ErrWrongKind{
		TypeName:        w.schemaType.Name(),
		MethodName:      "BeginList",
		AppropriateKind: datamodel.KindSet_JustList,
		ActualKind:      actualKind(w.schemaType),
	}
}

func (w *_assembler) AssignNull() error {
	if !w.nullable {
		return datamodel.ErrWrongKind{
			TypeName:   w.schemaType.Name(),
			MethodName: "AssignNull",
			// TODO
		}
	}
	w.val.Set(reflect.Zero(w.val.Type()))
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignBool(b bool) error {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Bool); err != nil {
		return err
	}
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		w.createNonPtrVal().Set(reflect.ValueOf(basicnode.NewBool(b)))
	} else {
		w.createNonPtrVal().SetBool(b)
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignInt(i int64) error {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Int); err != nil {
		return err
	}
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		w.createNonPtrVal().Set(reflect.ValueOf(basicnode.NewInt(i)))
	} else if kindUint[w.val.Kind()] {
		if i < 0 {
			// TODO: write a test
			return fmt.Errorf("bindnode: cannot assign negative integer to %s", w.val.Type())
		}
		w.createNonPtrVal().SetUint(uint64(i))
	} else {
		w.createNonPtrVal().SetInt(i)
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignFloat(f float64) error {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Float); err != nil {
		return err
	}
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		w.createNonPtrVal().Set(reflect.ValueOf(basicnode.NewFloat(f)))
	} else {
		w.createNonPtrVal().SetFloat(f)
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignString(s string) error {
	if err := compatibleKind(w.schemaType, datamodel.Kind_String); err != nil {
		return err
	}
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		w.createNonPtrVal().Set(reflect.ValueOf(basicnode.NewString(s)))
	} else {
		w.createNonPtrVal().SetString(s)
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignBytes(p []byte) error {
	if err := compatibleKind(w.schemaType, datamodel.Kind_Bytes); err != nil {
		return err
	}
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		w.createNonPtrVal().Set(reflect.ValueOf(basicnode.NewBytes(p)))
	} else {
		w.createNonPtrVal().SetBytes(p)
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignLink(link datamodel.Link) error {
	val := w.createNonPtrVal()
	// TODO: newVal.Type() panics if link==nil; add a test and fix.
	if _, ok := w.schemaType.(*schema.TypeAny); ok {
		val.Set(reflect.ValueOf(basicnode.NewLink(link)))
	} else if newVal := reflect.ValueOf(link); newVal.Type().AssignableTo(val.Type()) {
		// Directly assignable.
		val.Set(newVal)
	} else if newVal.Type() == goTypeCidLink && goTypeCid.AssignableTo(val.Type()) {
		// Unbox a cidlink.Link to assign to a go-cid.Cid value.
		newVal = newVal.FieldByName("Cid")
		val.Set(newVal)
	} else if actual := actualKind(w.schemaType); actual != datamodel.Kind_Link {
		// We're assigning a Link to a schema type that isn't a Link.
		return datamodel.ErrWrongKind{
			TypeName:        w.schemaType.Name(),
			MethodName:      "AssignLink",
			AppropriateKind: datamodel.KindSet_JustLink,
			ActualKind:      actualKind(w.schemaType),
		}
	} else {
		// The schema type is a Link, but we somehow can't assign to the Go value.
		// Almost certainly a bug; we should have verified for compatibility upfront.
		// fmt.Println(newVal.Type().ConvertibleTo(val.Type()))
		return fmt.Errorf("bindnode bug: AssignLink with %s argument can't be used on Go type %s",
			newVal.Type(), val.Type())
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_assembler) AssignNode(node datamodel.Node) error {
	// TODO: does this ever trigger?
	// newVal := reflect.ValueOf(node)
	// if newVal.Type().AssignableTo(w.val.Type()) {
	// 	w.val.Set(newVal)
	// 	return nil
	// }
	return datamodel.Copy(node, w)
}

func (w *_assembler) Prototype() datamodel.NodePrototype {
	return &_prototype{schemaType: w.schemaType, goType: w.val.Type()}
}

type _structAssembler struct {
	// TODO: embed _assembler?

	schemaType *schema.TypeStruct
	val        reflect.Value // non-pointer
	finish     func() error

	// TODO: more state checks

	// TODO: Consider if we could do this in a cheaper way,
	// such as looking at the reflect.Value directly.
	// If not, at least avoid an extra alloc.
	doneFields []bool

	// TODO: optimize for structs

	curKey _assembler

	nextIndex int // only used by repr.go
}

func (w *_structAssembler) AssembleKey() datamodel.NodeAssembler {
	w.curKey = _assembler{
		schemaType: schemaTypeString,
		val:        reflect.New(goTypeString).Elem(),
	}
	return &w.curKey
}

func (w *_structAssembler) AssembleValue() datamodel.NodeAssembler {
	// TODO: optimize this to do one lookup by name
	name := w.curKey.val.String()
	field := w.schemaType.Field(name)
	if field == nil {
		// TODO: should've been raised when the key was submitted instead.
		// TODO: should make well-typed errors for this.
		return _errorAssembler{fmt.Errorf("bindnode TODO: invalid key: %q is not a field in type %s", name, w.schemaType.Name())}
		// panic(schema.ErrInvalidKey{
		// 	TypeName: w.schemaType.Name(),
		// 	Key:      basicnode.NewString(name),
		// })
	}
	ftyp, ok := w.val.Type().FieldByName(fieldNameFromSchema(name))
	if !ok {
		// It is unfortunate this is not detected proactively earlier during bind.
		return _errorAssembler{fmt.Errorf("schema type %q has field %q, we expect go struct to have field %q", w.schemaType.Name(), field.Name(), fieldNameFromSchema(name))}
	}
	if len(ftyp.Index) > 1 {
		return _errorAssembler{fmt.Errorf("bindnode TODO: embedded fields")}
	}
	w.doneFields[ftyp.Index[0]] = true
	fval := w.val.FieldByIndex(ftyp.Index)
	if field.IsOptional() {
		fval.Set(reflect.New(fval.Type().Elem()))
		fval = fval.Elem()
	}
	// TODO: reuse same assembler for perf?
	return &_assembler{
		schemaType: field.Type(),
		val:        fval,
		nullable:   field.IsNullable(),
	}
}

func (w *_structAssembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	if err := w.AssembleKey().AssignString(k); err != nil {
		return nil, err
	}
	am := w.AssembleValue()
	return am, nil
}

func (w *_structAssembler) Finish() error {
	fields := w.schemaType.Fields()
	var missing []string
	for i, field := range fields {
		if !field.IsOptional() && !w.doneFields[i] {
			missing = append(missing, field.Name())
		}
	}
	if len(missing) > 0 {
		return schema.ErrMissingRequiredField{Missing: missing}
	}
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_structAssembler) KeyPrototype() datamodel.NodePrototype {
	// TODO: if the user provided their own schema with their own typesystem,
	// the schemaTypeString here may be using the wrong typesystem.
	return &_prototype{schemaType: schemaTypeString, goType: goTypeString}
}

func (w *_structAssembler) ValuePrototype(k string) datamodel.NodePrototype {
	panic("bindnode TODO: struct ValuePrototype")
}

type _errorAssembler struct {
	err error
}

func (w _errorAssembler) BeginMap(int64) (datamodel.MapAssembler, error)   { return nil, w.err }
func (w _errorAssembler) BeginList(int64) (datamodel.ListAssembler, error) { return nil, w.err }
func (w _errorAssembler) AssignNull() error                                { return w.err }
func (w _errorAssembler) AssignBool(bool) error                            { return w.err }
func (w _errorAssembler) AssignInt(int64) error                            { return w.err }
func (w _errorAssembler) AssignFloat(float64) error                        { return w.err }
func (w _errorAssembler) AssignString(string) error                        { return w.err }
func (w _errorAssembler) AssignBytes([]byte) error                         { return w.err }
func (w _errorAssembler) AssignLink(datamodel.Link) error                  { return w.err }
func (w _errorAssembler) AssignNode(datamodel.Node) error                  { return w.err }
func (w _errorAssembler) Prototype() datamodel.NodePrototype               { return nil }

type _mapAssembler struct {
	schemaType *schema.TypeMap
	keysVal    reflect.Value // non-pointer
	valuesVal  reflect.Value // non-pointer
	finish     func() error

	// TODO: more state checks

	curKey _assembler

	nextIndex int // only used by repr.go
}

func (w *_mapAssembler) AssembleKey() datamodel.NodeAssembler {
	w.curKey = _assembler{
		schemaType: w.schemaType.KeyType(),
		val:        reflect.New(w.valuesVal.Type().Key()).Elem(),
	}
	return &w.curKey
}

func (w *_mapAssembler) AssembleValue() datamodel.NodeAssembler {
	kval := w.curKey.val
	val := reflect.New(w.valuesVal.Type().Elem()).Elem()
	finish := func() error {
		// fmt.Println(kval.Interface(), val.Interface())

		// TODO: check for duplicates in keysVal
		w.keysVal.Set(reflect.Append(w.keysVal, kval))

		w.valuesVal.SetMapIndex(kval, val)
		return nil
	}
	return &_assembler{
		schemaType: w.schemaType.ValueType(),
		val:        val,
		nullable:   w.schemaType.ValueIsNullable(),
		finish:     finish,
	}
}

func (w *_mapAssembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	if err := w.AssembleKey().AssignString(k); err != nil {
		return nil, err
	}
	am := w.AssembleValue()
	return am, nil
}

func (w *_mapAssembler) Finish() error {
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_mapAssembler) KeyPrototype() datamodel.NodePrototype {
	return &_prototype{schemaType: w.schemaType.KeyType(), goType: w.valuesVal.Type().Key()}
}

func (w *_mapAssembler) ValuePrototype(k string) datamodel.NodePrototype {
	return &_prototype{schemaType: w.schemaType.ValueType(), goType: w.valuesVal.Type().Elem()}
}

type _listAssembler struct {
	schemaType *schema.TypeList
	val        reflect.Value // non-pointer
	finish     func() error
}

func (w *_listAssembler) AssembleValue() datamodel.NodeAssembler {
	goType := w.val.Type().Elem()
	// TODO: use a finish func to append
	w.val.Set(reflect.Append(w.val, reflect.New(goType).Elem()))
	return &_assembler{
		schemaType: w.schemaType.ValueType(),
		val:        w.val.Index(w.val.Len() - 1),
		nullable:   w.schemaType.ValueIsNullable(),
	}
}

func (w *_listAssembler) Finish() error {
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_listAssembler) ValuePrototype(idx int64) datamodel.NodePrototype {
	return &_prototype{schemaType: w.schemaType.ValueType(), goType: w.val.Type().Elem()}
}

type _unionAssembler struct {
	schemaType *schema.TypeUnion
	val        reflect.Value // non-pointer
	finish     func() error

	// TODO: more state checks

	curKey _assembler

	nextIndex int // only used by repr.go
}

func (w *_unionAssembler) AssembleKey() datamodel.NodeAssembler {
	w.curKey = _assembler{
		schemaType: schemaTypeString,
		val:        reflect.New(goTypeString).Elem(),
	}
	return &w.curKey
}

func (w *_unionAssembler) AssembleValue() datamodel.NodeAssembler {
	name := w.curKey.val.String()
	var idx int
	var mtyp schema.Type
	for i, member := range w.schemaType.Members() {
		if member.Name() == name {
			idx = i
			mtyp = member
			break
		}
	}
	if mtyp == nil {
		return _errorAssembler{fmt.Errorf("bindnode TODO: missing member %s in %s", name, w.schemaType.Name())}
		// return nil, datamodel.ErrInvalidKey{
		// 	TypeName: w.schemaType.Name(),
		// 	Key:      basicnode.NewString(name),
		// }
	}

	goType := w.val.Field(idx).Type().Elem()
	valPtr := reflect.New(goType)
	finish := func() error {
		// fmt.Println(kval.Interface(), val.Interface())
		unionSetMember(w.val, idx, valPtr)
		return nil
	}
	return &_assembler{
		schemaType: mtyp,
		val:        valPtr.Elem(),
		finish:     finish,
	}
}

func (w *_unionAssembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	if err := w.AssembleKey().AssignString(k); err != nil {
		return nil, err
	}
	am := w.AssembleValue()
	return am, nil
}

func (w *_unionAssembler) Finish() error {
	if w.finish != nil {
		if err := w.finish(); err != nil {
			return err
		}
	}
	return nil
}

func (w *_unionAssembler) KeyPrototype() datamodel.NodePrototype {
	return &_prototype{schemaType: schemaTypeString, goType: goTypeString}
}

func (w *_unionAssembler) ValuePrototype(k string) datamodel.NodePrototype {
	panic("bindnode TODO: union ValuePrototype")
}

type _structIterator struct {
	// TODO: support embedded fields?
	schemaType *schema.TypeStruct
	fields     []schema.StructField
	val        reflect.Value // non-pointer
	nextIndex  int

	// these are only used in repr.go
	reprEnd int
}

func (w *_structIterator) Next() (key, value datamodel.Node, _ error) {
	if w.Done() {
		return nil, nil, datamodel.ErrIteratorOverread{}
	}
	field := w.fields[w.nextIndex]
	val := w.val.Field(w.nextIndex)
	w.nextIndex++
	key = basicnode.NewString(field.Name())
	if field.IsOptional() {
		if val.IsNil() {
			return key, datamodel.Absent, nil
		}
		val = val.Elem()
	}
	if field.IsNullable() {
		if val.IsNil() {
			return key, datamodel.Null, nil
		}
		val = val.Elem()
	}
	if _, ok := field.Type().(*schema.TypeAny); ok {
		return key, nonPtrVal(val).Interface().(datamodel.Node), nil
	}
	node := &_node{
		schemaType: field.Type(),
		val:        val,
	}
	return key, node, nil
}

func (w *_structIterator) Done() bool {
	return w.nextIndex >= len(w.fields)
}

type _mapIterator struct {
	schemaType *schema.TypeMap
	keysVal    reflect.Value // non-pointer
	valuesVal  reflect.Value // non-pointer
	nextIndex  int
}

func (w *_mapIterator) Next() (key, value datamodel.Node, _ error) {
	if w.Done() {
		return nil, nil, datamodel.ErrIteratorOverread{}
	}
	goKey := w.keysVal.Index(w.nextIndex)
	val := w.valuesVal.MapIndex(goKey)
	w.nextIndex++

	key = &_node{
		schemaType: w.schemaType.KeyType(),
		val:        goKey,
	}
	if w.schemaType.ValueIsNullable() {
		if val.IsNil() {
			return key, datamodel.Null, nil
		}
		val = val.Elem()
	}
	if _, ok := w.schemaType.ValueType().(*schema.TypeAny); ok {
		return key, nonPtrVal(val).Interface().(datamodel.Node), nil
	}
	node := &_node{
		schemaType: w.schemaType.ValueType(),
		val:        val,
	}
	return key, node, nil
}

func (w *_mapIterator) Done() bool {
	return w.nextIndex >= w.keysVal.Len()
}

type _listIterator struct {
	schemaType *schema.TypeList
	val        reflect.Value // non-pointer
	nextIndex  int
}

func (w *_listIterator) Next() (index int64, value datamodel.Node, _ error) {
	if w.Done() {
		return 0, nil, datamodel.ErrIteratorOverread{}
	}
	idx := int64(w.nextIndex)
	val := w.val.Index(w.nextIndex)
	w.nextIndex++
	if w.schemaType.ValueIsNullable() {
		if val.IsNil() {
			return idx, datamodel.Null, nil
		}
		val = val.Elem()
	}
	if _, ok := w.schemaType.ValueType().(*schema.TypeAny); ok {
		return idx, nonPtrVal(val).Interface().(datamodel.Node), nil
	}
	return idx, &_node{schemaType: w.schemaType.ValueType(), val: val}, nil
}

func (w *_listIterator) Done() bool {
	return w.nextIndex >= w.val.Len()
}

type _unionIterator struct {
	// TODO: support embedded fields?
	schemaType *schema.TypeUnion
	members    []schema.Type
	val        reflect.Value // non-pointer

	done bool
}

func (w *_unionIterator) Next() (key, value datamodel.Node, _ error) {
	if w.Done() {
		return nil, nil, datamodel.ErrIteratorOverread{}
	}
	w.done = true

	haveIdx, mval := unionMember(w.val)
	if haveIdx < 0 {
		return nil, nil, fmt.Errorf("bindnode: union %s has no member", w.val.Type())
	}
	mtyp := w.members[haveIdx]

	node := &_node{
		schemaType: mtyp,
		val:        mval,
	}
	key = basicnode.NewString(mtyp.Name())
	return key, node, nil
}

func (w *_unionIterator) Done() bool {
	return w.done
}
