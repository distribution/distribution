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
