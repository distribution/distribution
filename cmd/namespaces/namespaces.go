package main

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
)

// Scope matches a namespace by prefix or glob.
type Scope string

// ParseScope checks that the string is a valid scope specification.
func ParseScope(s string) (Scope, error) {
	if s == "" {
		return "", fmt.Errorf("scope invalid: empty")
	}

	// check for valid glob syntax
	if _, err := path.Match(s, s); err != nil {
		return "", fmt.Errorf("scope %q invalid: %v", s, err)
	}

	// TODO(stevvooe): A validation regexp needs to be written to restrict the
	// scope to v2.RepositoryNameRegexp but also allow the occassional glob
	// character. The exact rules aren't currently clear but the validation
	// belongs here.

	scope := Scope(strings.TrimSpace(s))
	if !scope.isGlob() && s[len(s)-1] != '/' {
		return "", fmt.Errorf("scope invalid: %q must end with slash or be a glob", s)
	}

	return Scope(s), nil
}

// Match returns true if the name matches the scope.
func (s Scope) Match(name string) bool {
	// Check for an exact match, with a cleaned path component
	if path.Clean(string(s)) == path.Clean(name) {
		return true
	}

	// A simple prefix match is enough.
	if strings.HasPrefix(name, string(s)) {
		return true
	}

	matched, err := path.Match(string(s), name)
	if err != nil {
		panic(err) // all scopes must be valid globs.
	}

	return matched
}

var scopeGlobRegexp = regexp.MustCompile(`[*?\[\]]`) // if a string matches this, its a glob scope.

func (s Scope) isGlob() bool {
	return scopeGlobRegexp.MatchString(string(s))
}

type Action string

const (
	ActionPull  = "pull"
	ActionPush  = "push"
	ActionIndex = "index"
	ActionTrust = "trust"
	ActionAlias = "alias"

	// A namespace action declares a terminal node for resolution. Other nodes
	// may be queried for configuration but only namespace nodes will create
	// new image names.
	ActionNamespace = "namespace"
)

func ParseAction(s string) (Action, error) {
	switch s {
	case ActionPull:
		fallthrough
	case ActionPush:
		fallthrough
	case ActionIndex:
		fallthrough
	case ActionTrust:
		fallthrough
	case ActionNamespace:
		fallthrough
	case ActionAlias:
		return Action(s), nil
	default:
		return "", fmt.Errorf("action invalid: %q unsupported", s)
	}
}

// Entry defines the contents of a namespace configuration.
type Entry struct {
	// Scope identifies the namespaces to which the entry applies. If any name
	// matches scope, the entry applies.
	Scope Scope

	// Action is an opaque action field.
	Action Action

	// Args defines a set of arguments. The meaning and ordering of arguments
	// are dependent on the particular action.
	Args []string
}

func (entry Entry) expandAlias(name string) string {
	// TODO(stevvooe): This needs to move into a separate type.

	if entry.Action != ActionAlias {
		panic("not an alias")
	}

	scope := entry.Scope
	target := entry.Args[0]

	// if target ends with a slash, the name is appended to the target
	if strings.HasSuffix(target, "/") {
		return path.Join(target, name)
	}

	// otherwise, the scope is simply replaced in the name.
	return strings.Replace(name, path.Clean(string(scope)), path.Clean(target), 1)
}

func ParseEntry(s string) (Entry, error) {
	fields := strings.Fields(s)

	if len(fields) < 3 {
		return Entry{}, fmt.Errorf("entry invalid: must have <scope> <action> <args...>, %q", s)
	}

	scopeStr, actionStr, args := fields[0], fields[1], fields[2:]

	scope, err := ParseScope(scopeStr)
	if err != nil {
		return Entry{}, fmt.Errorf("entry invalid: %v", err)
	}

	action, err := ParseAction(actionStr)
	if err != nil {
		return Entry{}, fmt.Errorf("entry invalid: %v", err)
	}

	// globs are only allow on aliases
	if action != ActionAlias && scope.isGlob() {
		return Entry{}, fmt.Errorf("entry %q invalid: globs only allowed on aliases", s)
	}

	return Entry{
		Scope:  scope,
		Action: action,
		Args:   args,
	}, nil
}

// 1. Everything matches before aliases.
// 2. Exact and prefix matches take precedence over globs.
// 3. More specific globs take precedence over less specific globs.

// EntryLess returns true if a is less than b, providing a total ordering for
// entries for deterministically resolve and matching namespaces. The ordering
// determines precedence.
//
// The rules are the following:
// 		1. Records are first partitioned by action "alias" and not "alias".
//            "alias" actions always sort last.
//      2. Records that contain any glob character sort after those without
//         globs. This means exact matches and prefix matches take precedence.
//      3. Globs are sort in the reverse order of their length, effectively
//         making more specific globs match over less specific globs. (We may
//         need to work on these rules, perhaps, ratio of glob to non-glob
//         chars would be better).
//      4. If all the above fail to partition order, sort the entry lexically
//         using scope, action, args, in that order.
func EntryLess(a, b Entry) bool {
	if EntryEqual(a, b) {
		return false
	}

	// non-aliases always sort first.
	if a.Action != ActionAlias && b.Action == ActionAlias {
		return true
	}

	// non-globs always sort first.
	if !a.Scope.isGlob() && b.Scope.isGlob() {
		return true
	}

	if a.Scope.isGlob() && !b.Scope.isGlob() {
		return false
	}

	// globs sort in reverse order of length
	if a.Scope.isGlob() && b.Scope.isGlob() && a.Scope != b.Scope && len(a.Scope) != len(b.Scope) {
		// TODO(stevvooe): Scopes starting with "*" should always sort last.
		// Length should be enough for demonstrations.

		return len(a.Scope) > len(b.Scope)
	}

	// now, we start with scope and fallback to each field of the entry.
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}

	// fallback to action
	if a.Action != b.Action {
		return a.Action < b.Action
	}

	return strings.Join(a.Args, " ") < strings.Join(b.Args, " ")
}

func EntryEqual(a, b Entry) bool {
	v := a.Scope == b.Scope &&
		a.Action == b.Action &&
		strings.Join(a.Args, " ") == strings.Join(b.Args, " ")

	// log.Println(a, "==", b, v)
	return v

}

// Entries are the primary mechanism under which namespaces are transported,
// merged, stored and queried. They have a consistent sort order.
type Entries []Entry

// ParseEntries parses the serialized entries from a reader.
func ParseEntries(rd io.Reader) (Entries, error) {
	scanner := bufio.NewScanner(rd)

	var entries Entries
	var line int
	for scanner.Scan() {
		line++
		value := strings.TrimSpace(scanner.Text())

		if value == "" || value[0] == '#' {
			// TODO(stevvooe): We should probably save comment lines, if
			// possible. They would have to be part of the following entry.
			continue // skip line
		}

		entry, err := ParseEntry(value)
		if err != nil {
			return entries, fmt.Errorf("configuration invalid, line %d: %v", line, err)
		}

		entries.Add(entry)
	}

	return entries, scanner.Err()
}

func (entries Entries) Len() int           { return len(entries) }
func (entries Entries) Swap(i, j int)      { entries[i], entries[j] = entries[j], entries[i] }
func (entries Entries) Less(i, j int) bool { return EntryLess(entries[i], entries[j]) }

func (entries *Entries) Add(candidates ...Entry) error {
	for _, candidate := range candidates {
		i := entries.search(candidate)

		// fmt.Println("add", strconv.Quote(string(candidate.Scope)), strconv.Quote(string(candidate.Action)), i)
		if i < entries.Len() {
			if EntryEqual((*entries)[i], candidate) {
				// fmt.Println("add:fail", candidate, (*entries)[i])
				continue // don't insert duplicates
			}
		}

		entries.insertAt(i, candidate)
	}

	sort.Stable(*entries)
	return nil
}

func (entries *Entries) Remove(candidates ...Entry) error {
	for _, candidate := range candidates {
		// fmt.Println("remove", strconv.Quote(string(candidate.Scope)), strconv.Quote(string(candidate.Action)))
		i := entries.search(candidate)

		if i < entries.Len() {
			// Remove only identical entries.
			if EntryEqual((*entries)[i], candidate) {
				entries.removeAt(i)
			}
		}
	}

	return nil
}

// Find returns all the entries where namespace matches the scope. If no
// arguments are provided, all matches are returned.
func (entries Entries) Find(namespaces ...string) (Entries, error) {
	var found Entries

	for _, namespace := range namespaces {
		for _, entry := range entries {
			if !entry.Scope.Match(namespace) {
				continue
			}

			found.Add(entry)

			if entry.Action == ActionAlias {
				// Once we find a matching alias, the search is over. Only one
				// alias is allowed to match, in preference order.

				more, err := entries.Find(entry.expandAlias(namespace))
				if err != nil {
					return nil, err
				}

				found.Add(more...)
				return found, nil
			}
		}
	}

	if len(namespaces) == 0 {
		// empty args returns all.
		found.Add(entries...)
	}

	return found, nil
}

func (entries Entries) Resolve(name string) string {
	for _, entry := range entries {
		if !entry.Scope.Match(name) {
			continue
		}

		switch entry.Action {
		case ActionAlias:
			return entries.Resolve(entry.expandAlias(name))
		case ActionNamespace:
			return name // matched a namespace, okay to resolve
		default:
			// skip
		}
	}

	return name
}

// search returns the index at which entry would exist if it was already in
// the entries list or len(entries) if it would be last.
func (entries *Entries) search(entry Entry) int {
	return sort.Search(entries.Len(), func(i int) bool {
		return !EntryLess((*entries)[i], entry) // this may not be right.
	})
}

// insertAt places an entry at index i. Validity of i must be checked or it
// may panic expect if i == len(entries), in which case
func (entries *Entries) insertAt(i int, entry Entry) {
	if i == entries.Len() {
		// just append
		*entries = append(*entries, entry)
		return
	}

	// insert, possibly memory intensive
	*entries = append(*entries, Entry{})   // make an extra element
	copy((*entries)[i+1:], (*entries)[i:]) // slide everything over.
	(*entries)[i] = entry
}

// removeAt deletes the entry at index i.
func (entries *Entries) removeAt(i int) {
	(*entries) = append((*entries)[:i], (*entries)[i+1:]...)
}

type Discoverer interface {
	Discover(namespace string) (Entries, error)
}

// Manager defines the method set of an entity that lists and controls
// namespaces.
type Manager interface {
	Add(entries ...Entry) error
	Remove(entries ...Entry) error

	// Find provides access to the entries in this namespace manager. Given no
	// namespaces, the entire collection will be returned.
	Find(namespaces ...string) (Entries, error)

	Resolve(name string) string
}

// WriteManager writes the contents of manager to the writer. The format is
// acceptable for configuration files and command line output.
func WriteManager(wr io.Writer, m Manager) error {
	tw := tabwriter.NewWriter(wr, 8, 8, 4, ' ', 0)
	defer tw.Flush()

	all, err := m.Find()
	if err != nil {
		return err
	}

	for _, entry := range all {
		args := strings.Join(entry.Args, "\t")
		fmt.Fprintf(tw, "%s\t%s\t%s\n", entry.Scope, entry.Action, args)
	}

	return nil
}
