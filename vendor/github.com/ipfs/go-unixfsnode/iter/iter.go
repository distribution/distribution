package iter

import (
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
)

// PBLinkItr behaves like an list of links iterator, even thought he HAMT behavior is more complicated
type PBLinkItr interface {
	Next() (int64, dagpb.PBLink)
	Done() bool
}

type TransformNameFunc func(dagpb.String) dagpb.String

func NewUnixFSDirMapIterator(itr PBLinkItr, transformName TransformNameFunc) ipld.MapIterator {
	return &UnixFSDir__MapItr{itr, transformName}
}

// UnixFSDir__MapItr throught the links as if they were a map
// Note: for now it does return links with no name, where the key is just String("")
type UnixFSDir__MapItr struct {
	_substrate    PBLinkItr
	transformName TransformNameFunc
}

func (itr *UnixFSDir__MapItr) Next() (k ipld.Node, v ipld.Node, err error) {
	_, next := itr._substrate.Next()
	if next == nil {
		return nil, nil, ipld.ErrIteratorOverread{}
	}
	if next.FieldName().Exists() {
		name := next.FieldName().Must()
		if itr.transformName != nil {
			name = itr.transformName(name)
		}
		return name, next.FieldHash(), nil
	}
	nb := dagpb.Type.String.NewBuilder()
	err = nb.AssignString("")
	if err != nil {
		return nil, nil, err
	}
	s := nb.Build()
	return s, next.FieldHash(), nil
}

func (itr *UnixFSDir__MapItr) Done() bool {
	return itr._substrate.Done()
}

type UnixFSDir__Itr struct {
	_substrate    PBLinkItr
	transformName TransformNameFunc
}

func NewUnixFSDirIterator(itr PBLinkItr, transformName TransformNameFunc) *UnixFSDir__Itr {
	return &UnixFSDir__Itr{itr, transformName}
}
func (itr *UnixFSDir__Itr) Next() (k dagpb.String, v dagpb.Link) {
	_, next := itr._substrate.Next()
	if next == nil {
		return nil, nil
	}
	if next.FieldName().Exists() {
		name := next.FieldName().Must()
		if itr.transformName != nil {
			name = itr.transformName(name)
		}
		return name, next.FieldHash()
	}
	nb := dagpb.Type.String.NewBuilder()
	err := nb.AssignString("")
	if err != nil {
		return nil, nil
	}
	s := nb.Build()
	return s.(dagpb.String), next.FieldHash()
}

func (itr *UnixFSDir__Itr) Done() bool {
	return itr._substrate.Done()
}
