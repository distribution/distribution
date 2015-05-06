package namespace

import (
	"errors"
	"os"
)

// Resolver resolves a fully qualified name into
// a namespace configuration.
type Resolver interface {
	Resolve(name string) (*Entries, error)
}

// multiResolver does resolution across multiple resolvers
// in order of precedence. The next resolver is only used if
// a resolver returns no entries or there is a namespace entry
// with the targeted namespace matching the name of the scope
// being looked up.
type multiResolver struct {
	resolvers []Resolver
}

// NewMultiResolver returns a new resolver which attempts
// to use multiple resolvers in order to resolve a name
// to a set of entries. Resolution happens in the order
// the resolvers are passed, first having higher
// precendence.
func NewMultiResolver(resolver ...Resolver) Resolver {
	return &multiResolver{
		resolvers: resolver,
	}
}

func recursiveResolve(es *Entries, name string, resolvers []Resolver) (*Entries, error) {
	if len(resolvers) == 0 {
		return es, nil
	}
	resolved, err := resolvers[0].Resolve(name)
	if err != nil {
		return nil, err
	}
	if resolved != nil && len(resolved.entries) > 0 {
		for _, entry := range resolved.entries {
			// Recurse on namespace actions
			if entry.action == actionNamespace {
				for _, arg := range entry.args {
					sub, err := recursiveResolve(resolved, arg, resolvers[1:])
					if err != nil {
						return nil, err
					}
					resolved, err = resolved.Join(sub)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		return es.Join(resolved)
	}

	return recursiveResolve(es, name, resolvers[1:])
}

// Resolve resolves a name into a list of entries using multiple
// resolvers to collect the list. The resolved list is guaranteed
// to be unique even if multiple resolvers are called.
func (mr *multiResolver) Resolve(name string) (*Entries, error) {
	return recursiveResolve(NewEntries(), name, mr.resolvers)
}

// mimpleResolver is a resolver which uses a static set of entries
// to resolve a names based on the entry scope
type simpleResolver struct {
	entries     map[scope]*Entries
	prefixMatch bool
}

func (sr *simpleResolver) resolveEntries(es *Entries, name string) error {
	entries := sr.entries[scope(name)]
	if entries != nil {
		var extended []string
		for _, entry := range entries.entries {
			if err := es.Add(entry); err != nil {
				return err
			}
			if entry.action == actionNamespace {
				for _, arg := range entry.args {
					// When arg is not the name, also use additional scope
					if arg != name {
						scope, err := parseScope(arg)
						if err != nil {
							return err
						}
						if !scope.Contains(name) {
							return errors.New("invalid extension: must extend ancestor scope")
						}
						extended = append(extended, arg)
					}
				}
			}
		}
		for _, extend := range extended {
			if err := sr.resolveEntries(es, extend); err != nil {
				return err
			}
		}
	}

	// No results produced, fallback to any prefix matches
	if sr.prefixMatch && len(es.entries) == 0 {
		var longestScope string
		for s := range sr.entries {
			if s.Contains(name) && len(s.String()) > len(longestScope) {
				longestScope = s.String()
			}
		}
		if len(longestScope) > 0 {
			if err := sr.resolveEntries(es, longestScope); err != nil {
				return err
			}
		}
	}
	return nil
}

// Resolve resolves a name into a list of entries based on a static
// set of entries.
func (sr *simpleResolver) Resolve(name string) (*Entries, error) {
	entries := NewEntries()
	if err := sr.resolveEntries(entries, name); err != nil {
		return nil, err
	}
	return entries, nil
}

// NewSimpleResolver returns a resolver which will only match from
// the provided set of entries. If prefixMatch is set to true, then
// the resolver will return entries which have a prefix match if
// no other entries were found.
func NewSimpleResolver(base *Entries, prefixMatch bool) Resolver {
	entries := map[scope]*Entries{}
	for _, entry := range base.entries {
		scoped, ok := entries[entry.scope]
		if !ok {
			scoped = NewEntries()
			entries[entry.scope] = scoped
		}
		scoped.Add(entry)
	}
	return &simpleResolver{
		entries:     entries,
		prefixMatch: prefixMatch,
	}
}

// extendResolver extends the set of resolved entries only if
// entries were found.
type extendResolver struct {
	extendResolver Resolver
	baseResolver   Resolver
}

// Resolve resolves a name into a list of entries extending a
// list from another Resolver with a static set of entries.
func (er *extendResolver) Resolve(name string) (*Entries, error) {
	entries, err := er.baseResolver.Resolve(name)
	if err != nil {
		return nil, err
	}
	if len(entries.entries) > 0 {
		extended, err := er.extendResolver.Resolve(name)
		if err != nil {
			return nil, err
		}
		return entries.Join(extended)
	}
	return entries, nil
}

// NewExtendResolver returns a new Resolver which will extended the
// entries found through the given resolver with the given
// extended entries.
func NewExtendResolver(extension *Entries, resolver Resolver) Resolver {
	simple := NewSimpleResolver(extension, false)
	return &extendResolver{
		extendResolver: simple,
		baseResolver:   resolver,
	}
}

// NewDefaultFileResolver returns a new MultiResolver which uses
// the entries from the given file as the highest precendent then
// the following resolvers in the order they are given.
func NewDefaultFileResolver(namespaceFile string, resolvers ...Resolver) (Resolver, error) {
	// Read base entries from f.NamespaceFile
	nsf, err := os.Open(namespaceFile)
	if err != nil {
		return nil, err
	}

	entries, err := ParseEntries(nsf)
	if err != nil {
		return nil, err
	}
	resolvers = append([]Resolver{NewSimpleResolver(entries, true)}, resolvers...)
	return NewMultiResolver(resolvers...), nil

}

// GetRemoteEndpoints returns a list of remote endpoints from
// the set of entries.
func GetRemoteEndpoints(entries *Entries) ([]*RemoteEndpoint, error) {
	endpoints := []*RemoteEndpoint{}
	for _, entry := range entries.entries {
		switch entry.action {
		case actionIndex:
			fallthrough
		case actionPull:
			fallthrough
		case actionPush:
			endpoint, err := createEndpoint(entry)
			endpoints = append(endpoints, endpoint)
			if err != nil {
				return nil, err
			}
		}
	}
	return endpoints, nil
}
