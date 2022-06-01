package basicnode

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/mixins"
)

var (
	_ datamodel.Node          = &plainMap{}
	_ datamodel.NodePrototype = Prototype__Map{}
	_ datamodel.NodeBuilder   = &plainMap__Builder{}
	_ datamodel.NodeAssembler = &plainMap__Assembler{}
)

// plainMap is a concrete type that provides a map-kind datamodel.Node.
// It can contain any kind of value.
// plainMap is also embedded in the 'any' struct and usable from there.
type plainMap struct {
	m map[string]datamodel.Node // string key -- even if a runtime schema wrapper is using us for storage, we must have a comparable type here, and string is all we know.
	t []plainMap__Entry         // table for fast iteration, order keeping, and yielding pointers to enable alloc/conv amortization.
}

type plainMap__Entry struct {
	k plainString    // address of this used when we return keys as nodes, such as in iterators.  Need in one place to amortize shifts to heap when ptr'ing for iface.
	v datamodel.Node // identical to map values.  keeping them here simplifies iteration.  (in codegen'd maps, this position is also part of amortization, but in this implementation, that's less useful.)
	// note on alternate implementations: 'v' could also use the 'any' type, and thus amortize value allocations.  the memory size trade would be large however, so we don't, here.
}

// -- Node interface methods -->

func (plainMap) Kind() datamodel.Kind {
	return datamodel.Kind_Map
}
func (n *plainMap) LookupByString(key string) (datamodel.Node, error) {
	v, exists := n.m[key]
	if !exists {
		return nil, datamodel.ErrNotExists{Segment: datamodel.PathSegmentOfString(key)}
	}
	return v, nil
}
func (n *plainMap) LookupByNode(key datamodel.Node) (datamodel.Node, error) {
	ks, err := key.AsString()
	if err != nil {
		return nil, err
	}
	return n.LookupByString(ks)
}
func (plainMap) LookupByIndex(idx int64) (datamodel.Node, error) {
	return mixins.Map{TypeName: "map"}.LookupByIndex(0)
}
func (n *plainMap) LookupBySegment(seg datamodel.PathSegment) (datamodel.Node, error) {
	return n.LookupByString(seg.String())
}
func (n *plainMap) MapIterator() datamodel.MapIterator {
	return &plainMap_MapIterator{n, 0}
}
func (plainMap) ListIterator() datamodel.ListIterator {
	return nil
}
func (n *plainMap) Length() int64 {
	return int64(len(n.t))
}
func (plainMap) IsAbsent() bool {
	return false
}
func (plainMap) IsNull() bool {
	return false
}
func (plainMap) AsBool() (bool, error) {
	return mixins.Map{TypeName: "map"}.AsBool()
}
func (plainMap) AsInt() (int64, error) {
	return mixins.Map{TypeName: "map"}.AsInt()
}
func (plainMap) AsFloat() (float64, error) {
	return mixins.Map{TypeName: "map"}.AsFloat()
}
func (plainMap) AsString() (string, error) {
	return mixins.Map{TypeName: "map"}.AsString()
}
func (plainMap) AsBytes() ([]byte, error) {
	return mixins.Map{TypeName: "map"}.AsBytes()
}
func (plainMap) AsLink() (datamodel.Link, error) {
	return mixins.Map{TypeName: "map"}.AsLink()
}
func (plainMap) Prototype() datamodel.NodePrototype {
	return Prototype.Map
}

type plainMap_MapIterator struct {
	n   *plainMap
	idx int
}

func (itr *plainMap_MapIterator) Next() (k datamodel.Node, v datamodel.Node, _ error) {
	if itr.Done() {
		return nil, nil, datamodel.ErrIteratorOverread{}
	}
	k = &itr.n.t[itr.idx].k
	v = itr.n.t[itr.idx].v
	itr.idx++
	return
}
func (itr *plainMap_MapIterator) Done() bool {
	return itr.idx >= len(itr.n.t)
}

// -- NodePrototype -->

type Prototype__Map struct{}

func (Prototype__Map) NewBuilder() datamodel.NodeBuilder {
	return &plainMap__Builder{plainMap__Assembler{w: &plainMap{}}}
}

// -- NodeBuilder -->

type plainMap__Builder struct {
	plainMap__Assembler
}

func (nb *plainMap__Builder) Build() datamodel.Node {
	if nb.state != maState_finished {
		panic("invalid state: assembler must be 'finished' before Build can be called!")
	}
	return nb.w
}
func (nb *plainMap__Builder) Reset() {
	*nb = plainMap__Builder{}
	nb.w = &plainMap{}
}

// -- NodeAssembler -->

type plainMap__Assembler struct {
	w *plainMap

	ka plainMap__KeyAssembler
	va plainMap__ValueAssembler

	state maState
}
type plainMap__KeyAssembler struct {
	ma *plainMap__Assembler
}
type plainMap__ValueAssembler struct {
	ma *plainMap__Assembler
}

// maState is an enum of the state machine for a map assembler.
// (this might be something to export reusably, but it's also very much an impl detail that need not be seen, so, dubious.)
type maState uint8

const (
	maState_initial     maState = iota // also the 'expect key or finish' state
	maState_midKey                     // waiting for a 'finished' state in the KeyAssembler.
	maState_expectValue                // 'AssembleValue' is the only valid next step
	maState_midValue                   // waiting for a 'finished' state in the ValueAssembler.
	maState_finished                   // 'w' will also be nil, but this is a politer statement
)

func (na *plainMap__Assembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	if sizeHint < 0 {
		sizeHint = 0
	}
	// Allocate storage space.
	na.w.t = make([]plainMap__Entry, 0, sizeHint)
	na.w.m = make(map[string]datamodel.Node, sizeHint)
	// That's it; return self as the MapAssembler.  We already have all the right methods on this structure.
	return na, nil
}
func (plainMap__Assembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.MapAssembler{TypeName: "map"}.BeginList(0)
}
func (plainMap__Assembler) AssignNull() error {
	return mixins.MapAssembler{TypeName: "map"}.AssignNull()
}
func (plainMap__Assembler) AssignBool(bool) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignBool(false)
}
func (plainMap__Assembler) AssignInt(int64) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignInt(0)
}
func (plainMap__Assembler) AssignFloat(float64) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignFloat(0)
}
func (plainMap__Assembler) AssignString(string) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignString("")
}
func (plainMap__Assembler) AssignBytes([]byte) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignBytes(nil)
}
func (plainMap__Assembler) AssignLink(datamodel.Link) error {
	return mixins.MapAssembler{TypeName: "map"}.AssignLink(nil)
}
func (na *plainMap__Assembler) AssignNode(v datamodel.Node) error {
	// Sanity check assembler state.
	//  Update of state to 'finished' comes later; where exactly depends on if shortcuts apply.
	if na.state != maState_initial {
		panic("misuse")
	}
	// Copy the content.
	if v2, ok := v.(*plainMap); ok { // if our own type: shortcut.
		// Copy the structure by value.
		//  This means we'll have pointers into the same internal maps and slices;
		//   this is okay, because the Node type promises it's immutable, and we are going to instantly finish ourselves to also maintain that.
		// FIXME: the shortcut behaves differently than the long way: it discards any existing progress.  Doesn't violate immut, but is odd.
		*na.w = *v2
		na.state = maState_finished
		return nil
	}
	// If the above shortcut didn't work, resort to a generic copy.
	//  We call AssignNode for all the child values, giving them a chance to hit shortcuts even if we didn't.
	if v.Kind() != datamodel.Kind_Map {
		return datamodel.ErrWrongKind{TypeName: "map", MethodName: "AssignNode", AppropriateKind: datamodel.KindSet_JustMap, ActualKind: v.Kind()}
	}
	itr := v.MapIterator()
	for !itr.Done() {
		k, v, err := itr.Next()
		if err != nil {
			return err
		}
		if err := na.AssembleKey().AssignNode(k); err != nil {
			return err
		}
		if err := na.AssembleValue().AssignNode(v); err != nil {
			return err
		}
	}
	return na.Finish()
}
func (plainMap__Assembler) Prototype() datamodel.NodePrototype {
	return Prototype.Map
}

// -- MapAssembler -->

// AssembleEntry is part of conforming to MapAssembler, which we do on
// plainMap__Assembler so that BeginMap can just return a retyped pointer rather than new object.
func (ma *plainMap__Assembler) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	// Sanity check assembler state.
	//  Update of state comes after possible key rejection.
	if ma.state != maState_initial {
		panic("misuse")
	}
	// Check for dup keys; error if so.
	_, exists := ma.w.m[k]
	if exists {
		return nil, datamodel.ErrRepeatedMapKey{Key: plainString(k)}
	}
	ma.state = maState_midValue
	ma.w.t = append(ma.w.t, plainMap__Entry{k: plainString(k)})
	// Make value assembler valid by giving it pointer back to whole 'ma'; yield it.
	ma.va.ma = ma
	return &ma.va, nil
}

// AssembleKey is part of conforming to MapAssembler, which we do on
// plainMap__Assembler so that BeginMap can just return a retyped pointer rather than new object.
func (ma *plainMap__Assembler) AssembleKey() datamodel.NodeAssembler {
	// Sanity check, then update, assembler state.
	if ma.state != maState_initial {
		panic("misuse")
	}
	ma.state = maState_midKey
	// Make key assembler valid by giving it pointer back to whole 'ma'; yield it.
	ma.ka.ma = ma
	return &ma.ka
}

// AssembleValue is part of conforming to MapAssembler, which we do on
// plainMap__Assembler so that BeginMap can just return a retyped pointer rather than new object.
func (ma *plainMap__Assembler) AssembleValue() datamodel.NodeAssembler {
	// Sanity check, then update, assembler state.
	if ma.state != maState_expectValue {
		panic("misuse")
	}
	ma.state = maState_midValue
	// Make value assembler valid by giving it pointer back to whole 'ma'; yield it.
	ma.va.ma = ma
	return &ma.va
}

// Finish is part of conforming to MapAssembler, which we do on
// plainMap__Assembler so that BeginMap can just return a retyped pointer rather than new object.
func (ma *plainMap__Assembler) Finish() error {
	// Sanity check, then update, assembler state.
	if ma.state != maState_initial {
		panic("misuse")
	}
	ma.state = maState_finished
	// validators could run and report errors promptly, if this type had any.
	return nil
}
func (plainMap__Assembler) KeyPrototype() datamodel.NodePrototype {
	return Prototype__String{}
}
func (plainMap__Assembler) ValuePrototype(_ string) datamodel.NodePrototype {
	return Prototype.Any
}

// -- MapAssembler.KeyAssembler -->

func (plainMap__KeyAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	return mixins.StringAssembler{TypeName: "string"}.BeginMap(0)
}
func (plainMap__KeyAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	return mixins.StringAssembler{TypeName: "string"}.BeginList(0)
}
func (plainMap__KeyAssembler) AssignNull() error {
	return mixins.StringAssembler{TypeName: "string"}.AssignNull()
}
func (plainMap__KeyAssembler) AssignBool(bool) error {
	return mixins.StringAssembler{TypeName: "string"}.AssignBool(false)
}
func (plainMap__KeyAssembler) AssignInt(int64) error {
	return mixins.StringAssembler{TypeName: "string"}.AssignInt(0)
}
func (plainMap__KeyAssembler) AssignFloat(float64) error {
	return mixins.StringAssembler{TypeName: "string"}.AssignFloat(0)
}
func (mka *plainMap__KeyAssembler) AssignString(v string) error {
	// Check for dup keys; error if so.
	//  (And, backtrack state to accepting keys again so we don't get eternally wedged here.)
	_, exists := mka.ma.w.m[v]
	if exists {
		mka.ma.state = maState_initial
		mka.ma = nil // invalidate self to prevent further incorrect use.
		return datamodel.ErrRepeatedMapKey{Key: plainString(v)}
	}
	// Assign the key into the end of the entry table;
	//  we'll be doing map insertions after we get the value in hand.
	//  (There's no need to delegate to another assembler for the key type,
	//   because we're just at Data Model level here, which only regards plain strings.)
	mka.ma.w.t = append(mka.ma.w.t, plainMap__Entry{})
	mka.ma.w.t[len(mka.ma.w.t)-1].k = plainString(v)
	// Update parent assembler state: clear to proceed.
	mka.ma.state = maState_expectValue
	mka.ma = nil // invalidate self to prevent further incorrect use.
	return nil
}
func (plainMap__KeyAssembler) AssignBytes([]byte) error {
	return mixins.StringAssembler{TypeName: "string"}.AssignBytes(nil)
}
func (plainMap__KeyAssembler) AssignLink(datamodel.Link) error {
	return mixins.StringAssembler{TypeName: "string"}.AssignLink(nil)
}
func (mka *plainMap__KeyAssembler) AssignNode(v datamodel.Node) error {
	vs, err := v.AsString()
	if err != nil {
		return fmt.Errorf("cannot assign non-string node into map key assembler") // FIXME:errors: this doesn't quite fit in ErrWrongKind cleanly; new error type?
	}
	return mka.AssignString(vs)
}
func (plainMap__KeyAssembler) Prototype() datamodel.NodePrototype {
	return Prototype__String{}
}

// -- MapAssembler.ValueAssembler -->

func (mva *plainMap__ValueAssembler) BeginMap(sizeHint int64) (datamodel.MapAssembler, error) {
	ma := plainMap__ValueAssemblerMap{}
	ma.ca.w = &plainMap{}
	ma.p = mva.ma
	_, err := ma.ca.BeginMap(sizeHint)
	return &ma, err
}
func (mva *plainMap__ValueAssembler) BeginList(sizeHint int64) (datamodel.ListAssembler, error) {
	la := plainMap__ValueAssemblerList{}
	la.ca.w = &plainList{}
	la.p = mva.ma
	_, err := la.ca.BeginList(sizeHint)
	return &la, err
}
func (mva *plainMap__ValueAssembler) AssignNull() error {
	return mva.AssignNode(datamodel.Null)
}
func (mva *plainMap__ValueAssembler) AssignBool(v bool) error {
	vb := plainBool(v)
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignInt(v int64) error {
	vb := plainInt(v)
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignFloat(v float64) error {
	vb := plainFloat(v)
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignString(v string) error {
	vb := plainString(v)
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignBytes(v []byte) error {
	vb := plainBytes(v)
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignLink(v datamodel.Link) error {
	vb := plainLink{v}
	return mva.AssignNode(&vb)
}
func (mva *plainMap__ValueAssembler) AssignNode(v datamodel.Node) error {
	l := len(mva.ma.w.t) - 1
	mva.ma.w.t[l].v = v
	mva.ma.w.m[string(mva.ma.w.t[l].k)] = v
	mva.ma.state = maState_initial
	mva.ma = nil // invalidate self to prevent further incorrect use.
	return nil
}
func (plainMap__ValueAssembler) Prototype() datamodel.NodePrototype {
	return Prototype.Any
}

type plainMap__ValueAssemblerMap struct {
	ca plainMap__Assembler
	p  *plainMap__Assembler // pointer back to parent, for final insert and state bump
}

// we briefly state only the methods we need to delegate here.
// just embedding plainMap__Assembler also behaves correctly,
//  but causes a lot of unnecessary autogenerated functions in the final binary.

func (ma *plainMap__ValueAssemblerMap) AssembleEntry(k string) (datamodel.NodeAssembler, error) {
	return ma.ca.AssembleEntry(k)
}
func (ma *plainMap__ValueAssemblerMap) AssembleKey() datamodel.NodeAssembler {
	return ma.ca.AssembleKey()
}
func (ma *plainMap__ValueAssemblerMap) AssembleValue() datamodel.NodeAssembler {
	return ma.ca.AssembleValue()
}
func (plainMap__ValueAssemblerMap) KeyPrototype() datamodel.NodePrototype {
	return Prototype__String{}
}
func (plainMap__ValueAssemblerMap) ValuePrototype(_ string) datamodel.NodePrototype {
	return Prototype.Any
}

func (ma *plainMap__ValueAssemblerMap) Finish() error {
	if err := ma.ca.Finish(); err != nil {
		return err
	}
	w := ma.ca.w
	ma.ca.w = nil
	return ma.p.va.AssignNode(w)
}

type plainMap__ValueAssemblerList struct {
	ca plainList__Assembler
	p  *plainMap__Assembler // pointer back to parent, for final insert and state bump
}

// we briefly state only the methods we need to delegate here.
// just embedding plainList__Assembler also behaves correctly,
//  but causes a lot of unnecessary autogenerated functions in the final binary.

func (la *plainMap__ValueAssemblerList) AssembleValue() datamodel.NodeAssembler {
	return la.ca.AssembleValue()
}
func (plainMap__ValueAssemblerList) ValuePrototype(_ int64) datamodel.NodePrototype {
	return Prototype.Any
}

func (la *plainMap__ValueAssemblerList) Finish() error {
	if err := la.ca.Finish(); err != nil {
		return err
	}
	w := la.ca.w
	la.ca.w = nil
	return la.p.va.AssignNode(w)
}
