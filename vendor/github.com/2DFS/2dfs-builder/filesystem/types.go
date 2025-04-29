package filesystem

import (
	"encoding/json"
	"fmt"
	"sync"
)

type Allotment struct {
	Row    int    `json:"row"`
	Col    int    `json:"col"`
	Digest string `json:"digest"`
	DiffID string `json:"diffid"`
}

type Cols struct {
	Allotments    []Allotment `json:"allotments"`
	TotAllotments int         `json:"allotments_size"`
}

type TwoDFilesystem struct {
	Rows    []Cols `json:"rows"`
	TotRows int    `json:"rows_size"`
	Owner   string `json:"owner"`
	mtx     sync.Mutex
}

type AllotmentManifest struct {
	Src StringList `json:"src"`
	Dst StringList `json:"dst"`
	Row int        `json:"row"`
	Col int        `json:"col"`
}

type TwoDFsManifest struct {
	Allotments []AllotmentManifest `json:"allotments"`
}

type Field interface {
	// AddAllotment creates the given allotment to the 2d FileSystem
	AddAllotment(allotment Allotment) Field
	// Marshal gives a marshalled filesystem as string
	Marshal() string
	// Unmarshal Given a string marshaled from TwoDFilesystem returns a Field object
	Unmarshal(string) (Field, error)
	// IterateAllotments iterates over all allotments in the filesystem
	IterateAllotments() chan Allotment
}

// StringOrStringList represents a type that wraps a string list. It unmarshals as list even a single string.
type StringList struct {
	List []string
}

// UnmarshalJSON custom unmarshaler for StringList
func (s *StringList) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string and convert it to list
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.List = []string{str}
		return nil
	}

	// Try to unmarshal as a list of strings
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		s.List = list
		return nil
	}

	return fmt.Errorf("invalid type for StringOrStringList")
}
