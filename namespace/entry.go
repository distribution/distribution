package namespace

import (
	"fmt"
	"sort"
	"strings"

	"github.com/docker/distribution"
)

const (
	actionPull      = "pull"
	actionPush      = "push"
	actionIndex     = "index"
	actionNamespace = "namespace"
)

func checkAction(s string) error {
	switch s {
	case actionPull:
		fallthrough
	case actionPush:
		fallthrough
	case actionIndex:
		fallthrough
	case actionNamespace:
		return nil
	default:
		return fmt.Errorf("action invalid: %q unsupported", s)
	}
}

// Entry defines the contents of a namespace configuration.
type Entry struct {
	// scope identifies the namespaces to which the entry applies. If any name
	// matches scope, the entry may apply.
	scope scope

	// action is an opaque action field.
	action string

	// args defines a set of arguments. The meaning and ordering of arguments
	// are dependent on the particular action.
	args []string
}

// NewEntry creates a new entry with the given scope and action.  If the scope,
// action, or action arguments are invalid, this function will return an error.
func NewEntry(scopeStr, action string, args ...string) (Entry, error) {
	scp, err := parseScope(scopeStr)
	if err != nil {
		return Entry{}, fmt.Errorf("entry invalid: %v", err)
	}

	if err := checkAction(action); err != nil {
		return Entry{}, fmt.Errorf("entry invalid: %v", err)
	}
	return Entry{
		scope:  scp,
		action: action,
		args:   args,
	}, nil
}

// Scope returns the scope of the entry.
func (e Entry) Scope() distribution.Scope {
	return e.scope
}

// Action returns the action of the entry.
func (e Entry) Action() string {
	return e.action
}

// Args return the arguments for the action of the entry.
func (e Entry) Args() []string {
	return e.args
}

// entryLess returns true if a is less than b, providing a total ordering for
// entries for deterministically resolve and matching namespaces. The entries
// are sorted lexically using scope, action, args, in that order.
func entryLess(a, b Entry) bool {
	if entryEqual(a, b) {
		return false
	}

	// now, we start with scope and fallback to each field of the entry.
	if a.scope != b.scope {
		return a.scope < b.scope
	}

	// fallback to action
	if a.action != b.action {
		return a.action < b.action
	}

	return strings.Join(a.args, " ") < strings.Join(b.args, " ")
}

func entryEqual(a, b Entry) bool {
	v := a.scope == b.scope &&
		a.action == b.action &&
		strings.Join(a.args, " ") == strings.Join(b.args, " ")

	// log.Println(a, "==", b, v)
	return v
}

// Entries are the primary mechanism under which namespaces are transported,
// merged, stored and queried. This represents a unique set of entries.
type Entries struct {
	entries entries
}

// NewEntries returns an empty entries object.
func NewEntries() *Entries {
	return &Entries{
		entries: make(entries, 0, 8),
	}
}

type entries []Entry

func (e entries) Len() int           { return len(e) }
func (e entries) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e entries) Less(i, j int) bool { return entryLess(e[i], e[j]) }

// Add adds an entry to the entries set. If the entry is a duplicate of
// an entry already in the set, nothing will be inserted and nil will be
// returned.
func (es *Entries) Add(entry Entry) error {
	i := es.search(entry)

	if i < es.entries.Len() {
		if entryEqual(es.entries[i], entry) {
			return nil
		}
	}

	es.insertAt(i, entry)

	return nil
}

// Remove removes an entry from the entries set which matches the given
// entry. The entry must match exactly scope, action, and arguments.
func (es *Entries) Remove(entry Entry) error {
	i := es.search(entry)

	if i < es.entries.Len() {
		// Remove only identical entries.
		if entryEqual(es.entries[i], entry) {
			es.removeAt(i)
		}
	}

	return nil
}

// Find returns all the entries where namespace matches the scope. If no
// arguments are provided, all matches are returned.
func (es *Entries) Find(namespaces ...string) (*Entries, error) {
	if len(namespaces) == 0 {
		return es, nil
	}

	found := &Entries{
		entries: make(entries, 0),
	}

	for _, namespace := range namespaces {
		for _, entry := range es.entries {
			if !entry.Scope().Contains(namespace) {
				continue
			}

			found.Add(entry)
		}
	}

	return found, nil
}

// Join joins two entries sets together. The returned entries will
// contain no duplicates between the sets.
func (es *Entries) Join(other *Entries) (*Entries, error) {
	c := &Entries{entries: es.entries}
	if other == nil {
		return c, nil
	}
	for _, entry := range other.entries {
		if err := c.Add(entry); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// search returns the index at which entry would exist if it was already in
// the entries list or len(entries) if it would be last.
func (es *Entries) search(entry Entry) int {
	return sort.Search(es.entries.Len(), func(i int) bool {
		return !entryLess(es.entries[i], entry) // this may not be right.
	})
}

// insertAt places an entry at index i. Validity of i must be checked or it
// may panic expect if i == len(entries), in which case
func (es *Entries) insertAt(i int, entry Entry) {
	if i == es.entries.Len() {
		// just append
		es.entries = append(es.entries, entry)
		return
	}

	// insert, possibly memory intensive
	es.entries = append(es.entries, Entry{}) // make an extra element
	copy(es.entries[i+1:], es.entries[i:])   // slide everything over.
	es.entries[i] = entry
}

// removeAt deletes the entry at index i.
func (es *Entries) removeAt(i int) {
	es.entries = append(es.entries[:i], es.entries[i+1:]...)
}
