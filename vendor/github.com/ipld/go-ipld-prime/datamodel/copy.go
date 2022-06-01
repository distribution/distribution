package datamodel

import (
	"fmt"
)

// Copy does an explicit shallow copy of a Node's data into a NodeAssembler.
//
// This can be used to flip data from one memory layout to another
// (for example, from basicnode to using using bindnode,
// or to codegenerated node implementations,
// or to or from ADL nodes, etc).
//
// The copy is implemented by ranging over the contents if it's a recursive kind,
// and for each of them, using `AssignNode` on the child values;
// for scalars, it's just calling the appropriate `Assign*` method.
//
// Many NodeAssembler implementations use this as a fallback behavior in their
// `AssignNode` method (that is, they call to this function after all other special
// faster shortcuts they might prefer to employ, such as direct struct copying
// if they share internal memory layouts, etc, have been tried already).
//
func Copy(n Node, na NodeAssembler) error {
	switch n.Kind() {
	case Kind_Null:
		if n.IsAbsent() {
			return fmt.Errorf("copying an absent node makes no sense")
		}
		return na.AssignNull()
	case Kind_Bool:
		v, err := n.AsBool()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsBool method returned %w", n.Kind(), err)
		}
		return na.AssignBool(v)
	case Kind_Int:
		v, err := n.AsInt()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsInt method returned %w", n.Kind(), err)
		}
		return na.AssignInt(v)
	case Kind_Float:
		v, err := n.AsFloat()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsFloat method returned %w", n.Kind(), err)
		}
		return na.AssignFloat(v)
	case Kind_String:
		v, err := n.AsString()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsString method returned %w", n.Kind(), err)
		}
		return na.AssignString(v)
	case Kind_Bytes:
		v, err := n.AsBytes()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsBytes method returned %w", n.Kind(), err)
		}
		return na.AssignBytes(v)
	case Kind_Link:
		v, err := n.AsLink()
		if err != nil {
			return fmt.Errorf("node violated contract: promised to be %v kind, but AsLink method returned %w", n.Kind(), err)
		}
		return na.AssignLink(v)
	case Kind_Map:
		ma, err := na.BeginMap(n.Length())
		if err != nil {
			return err
		}
		itr := n.MapIterator()
		for !itr.Done() {
			k, v, err := itr.Next()
			if err != nil {
				return err
			}
			if v.IsAbsent() {
				continue
			}
			if err := ma.AssembleKey().AssignNode(k); err != nil {
				return err
			}
			if err := ma.AssembleValue().AssignNode(v); err != nil {
				return err
			}
		}
		return ma.Finish()
	case Kind_List:
		la, err := na.BeginList(n.Length())
		if err != nil {
			return err
		}
		itr := n.ListIterator()
		for !itr.Done() {
			_, v, err := itr.Next()
			if err != nil {
				return err
			}
			if v.IsAbsent() {
				continue
			}
			if err := la.AssembleValue().AssignNode(v); err != nil {
				return err
			}
		}
		return la.Finish()
	default:
		return fmt.Errorf("node has invalid kind %v", n.Kind())
	}
}
