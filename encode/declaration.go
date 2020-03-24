package encode

import "strings"

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
		if v == '1' {
			d.Encodings[i] = true
		} else {
			d.Encodings[i] = false
		}
	}
	return d
}
