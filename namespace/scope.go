package namespace

import (
	"fmt"
	"path"
	"strings"
)

// scope matches a namespace by prefix or glob.
type scope string

// parseScope checks that the string is a valid scope specification.
func parseScope(s string) (scope, error) {
	if s == "" {
		return "", fmt.Errorf("scope invalid: empty")
	}

	// TODO(dmcgowan): Add support for exact and single component matching
	// scope/** - Any prefix match
	// scope/* - Single element match
	// scope/ - Any prefix match (same as /**)
	// scope - Exact match

	// TODO(stevvooe): A validation regexp needs to be written to restrict the
	// scope to v2.RepositoryNameRegexp but also allow the occassional glob
	// character. The exact rules aren't currently clear but the validation // belongs here.

	return scope(s), nil
}

// Contains returns true if the name matches the scope.
func (s scope) Contains(name string) bool {
	cleanScope := path.Clean(string(s))
	cleanName := path.Clean(name)
	// Check for an exact match, with a cleaned path component
	if cleanScope == cleanName {
		return true
	}

	// A simple prefix match is enough.
	if strings.HasPrefix(cleanName, cleanScope+"/") {
		return true
	}

	return false
}

func (s scope) String() string {
	return string(s)
}
