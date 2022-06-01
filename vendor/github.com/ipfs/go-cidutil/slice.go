package cidutil

import (
	"github.com/ipfs/go-cid"
	"sort"
)

// Slice is a convenience type for sorting CIDs
type Slice []cid.Cid

func (s Slice) Len() int {
	return len(s)
}

func (s Slice) Less(i, j int) bool {
	return s[i].KeyString() < s[j].KeyString()
}

func (s Slice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s Slice) Sort() {
	sort.Sort(s)
}

// Sort sorts a slice of CIDs
func Sort(s []cid.Cid) {
	Slice(s).Sort()
}
