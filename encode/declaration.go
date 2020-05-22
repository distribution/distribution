package encode

import (
	"strings"

	"github.com/golang-collections/collections/set"
)

//Declaration represents the class which tells
//if the block at index i actually exists
type Declaration struct {
	Encodings []bool
}

// String will fecth the declaration as a string
func (d *Declaration) String() string {
	var str strings.Builder
	for _, exist := range d.Encodings {
		if exist {
			str.WriteString("1")
		} else {
			str.WriteString("0")
		}
	}

	return str.String()
}

// NewDeclarationFromString will generate a declaration
// object from a string
func NewDeclarationFromString(rawDeclaration string) Declaration {
	var d Declaration
	d.Encodings = make([]bool, len(rawDeclaration))
	for i, v := range rawDeclaration {
		d.Encodings[i] = (v == '1')
	}
	return d
}

//NewDeclarationForNode Will generate a declaration from a recipe and the list of blocks in Set
func NewDeclarationForNode(recipe Recipe, blocksInNode set.Set) Declaration {
	var d Declaration
	d.Encodings = make([]bool, len(recipe.Keys))
	for i, v := range recipe.Keys {
		d.Encodings[i] = blocksInNode.Has(interface{}(v))
	}
	return d
}
