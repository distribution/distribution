package filesystem

import "sync"

type Allotment struct {
	Row      int    `json:"row"`
	Col      int    `json:"col"`
	Digest   string `json:"digest"`
	DiffID   string `json:"diffid"`
	FileName string `json:"filename"`
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
	Src string `json:"src"`
	Dst string `json:"dst"`
	Row int    `json:"row"`
	Col int    `json:"col"`
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
