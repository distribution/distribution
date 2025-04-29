package filesystem

import (
	"encoding/json"
	"sync"
)

func GetField() Field {
	return &TwoDFilesystem{
		Rows:    make([]Cols, 0),
		TotRows: 0,
		mtx:     sync.Mutex{},
	}
}

func (f *TwoDFilesystem) AddAllotment(allotment Allotment) Field {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.genAllotments(allotment.Row, allotment.Col)
	f.Rows[allotment.Row].Allotments[allotment.Col].Digest = allotment.Digest
	f.Rows[allotment.Row].Allotments[allotment.Col].DiffID = allotment.DiffID
	return f
}

func (f *TwoDFilesystem) Marshal() string {
	result, _ := json.Marshal(f)
	return string(result)
}

func (f *TwoDFilesystem) Unmarshal(data string) (Field, error) {
	fs := TwoDFilesystem{}
	err := json.Unmarshal([]byte(data), &fs)
	if err != nil {
		return nil, err
	}
	return &fs, nil
}

// genRows generates (if necessary) the rows from 0 to n. If some rows exist already, they are not generated.
// E.g., f.TotRows=0, genRows(0) -> generates row[0]
// E.g., f.TotRows=4, genRows(7) -> generates row[4],row[5],row[6],row[7]
// E.g., f.TotRows=4, genRows(3) -> no new rows generated
func (f *TwoDFilesystem) genRows(n int) {
	if n > (f.TotRows - 1) {
		for i := f.TotRows; i <= n; i++ {
			f.Rows = append(f.Rows, Cols{
				Allotments:    make([]Allotment, 0),
				TotAllotments: 0,
			})
		}
		f.TotRows = n + 1
	}
}

// genAllotment generates (if necessary) the allotments from 0 to n on "row". If some allotments exist already, they are not generated.
func (f *TwoDFilesystem) genAllotments(row int, n int) {
	f.genRows(row)
	if n > (f.Rows[row].TotAllotments - 1) {
		for i := f.Rows[row].TotAllotments; i <= n; i++ {
			f.Rows[row].Allotments = append(f.Rows[row].Allotments, Allotment{
				Row:    row,
				Col:    i,
				Digest: "",
				DiffID: "",
			})
		}
		f.Rows[row].TotAllotments = n + 1
	}
}

// IterateAllotments iterates over all allotments in the filesystem
func (f *TwoDFilesystem) IterateAllotments() chan Allotment {
	c := make(chan Allotment)
	go func() {
		for _, row := range f.Rows {
			for _, allotment := range row.Allotments {
				c <- allotment
			}
		}
		close(c)
	}()
	return c
}
