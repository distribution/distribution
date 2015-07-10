package reference

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	// RepositoryNameTotalLengthMax is the maximum total number of characters in a repository name.
	RepositoryNameTotalLengthMax = 255
)

// RepositoryNameComponentRegexp restricts registry path component names to
// start with at least one letter or number, with following parts able to
// be separated by one period, dash or underscore.
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-zA-Z0-9]+(?:[._-][a-z0-9]+)*`)

// RepositoryNameComponentAnchoredRegexp is the version of
// RepositoryNameComponentRegexp which must completely match the content
var RepositoryNameComponentAnchoredRegexp = regexp.MustCompile(`^` + RepositoryNameComponentRegexp.String() + `$`)

// RepositoryNameHostnameRegexp restricts the registry hostname component of a repository name to
// start with a component as defined by RepositoryNameComponentRegexp and followed by an optional port.
var RepositoryNameHostnameRegexp = regexp.MustCompile(RepositoryNameComponentRegexp.String() + `(?::[0-9]+)?`)

// RepositoryNameHostnameAnchoredRegexp is the version of
// RepositoryNameHostnameRegexp which must completely match the content.
var RepositoryNameHostnameAnchoredRegexp = regexp.MustCompile(`^` + RepositoryNameHostnameRegexp.String() + `$`)

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow
// multiple path components, separated by a forward slash.
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameHostnameRegexp.String() + `/)?(?:` + RepositoryNameComponentRegexp.String() + `/)*` + RepositoryNameComponentRegexp.String())

var (
	// ErrRepositoryNameEmpty is returned for empty, invalid repository names.
	ErrRepositoryNameEmpty = errors.New("repository name must have at least one component")

	// ErrRepositoryNameMissingHostname is returned when a repository name
	// does not start with a hostname
	ErrRepositoryNameMissingHostname = errors.New("repository name must start with a hostname")

	// ErrRepositoryNameHostnameInvalid is returned when a repository name
	// does not match RepositoryNameHostnameRegexp
	ErrRepositoryNameHostnameInvalid = fmt.Errorf("repository name must match %q", RepositoryNameHostnameRegexp.String())

	// ErrRepositoryNameLong is returned when a repository name is longer than
	// RepositoryNameTotalLengthMax
	ErrRepositoryNameLong = fmt.Errorf("repository name must not be more than %v characters", RepositoryNameTotalLengthMax)

	// ErrRepositoryNameComponentInvalid is returned when a repository name does
	// not match RepositoryNameComponentRegexp
	ErrRepositoryNameComponentInvalid = fmt.Errorf("repository name component must match %q", RepositoryNameComponentRegexp.String())
)

// Repository represents a reference to a Repository.
type Repository struct {
	// Hostname refers to the registry hostname where the repository resides.
	Hostname string
	// Name is a slash (`/`) separated list of string components.
	Name string
}

// String returns the string representation of a repository.
func (r Repository) String() string {
	// Hostname is not supposed to be empty, but let's be nice.
	if len(r.Hostname) == 0 {
		return r.Name
	}
	return r.Hostname + "/" + r.Name
}

// Validate ensures the repository name is valid for use in the
// registry. This function accepts a superset of what might be accepted by
// docker core or docker hub. If the name does not pass validation, an error,
// describing the conditions, is returned.
//
// Effectively, the name should comply with the following grammar:
//
//	repository			:= hostname ['/' component]+
//	hostname 			:= component [':' port-number]
//	component			:= alpha-numeric [separator alpha-numeric]*
// 	alpha-numeric			:= /[a-zA-Z0-9]+/
//	separator			:= /[._-]/
//	port-number			:= /[0-9]+/
//
// The result of the production should be limited to 255 characters.
func (r Repository) Validate() error {
	n := len(r.String())
	switch {
	case n == 0:
		return ErrRepositoryNameEmpty
	case n > RepositoryNameTotalLengthMax:
		return ErrRepositoryNameLong
	case len(r.Hostname) <= 0:
		return ErrRepositoryNameMissingHostname
	case !RepositoryNameHostnameAnchoredRegexp.MatchString(r.Hostname):
		return ErrRepositoryNameHostnameInvalid
	}

	components := r.Name
	for {
		var component string
		sep := strings.Index(components, "/")
		if sep >= 0 {
			component = components[:sep]
			components = components[sep+1:]
		} else { // if no more slashes
			component = components
			components = ""
		}
		if !RepositoryNameComponentAnchoredRegexp.MatchString(component) {
			return ErrRepositoryNameComponentInvalid
		}
		if sep < 0 {
			return nil
		}
	}
}

// NewRepository returns a valid Repository from an input string representing
// the canonical form of a repository name.
// If the validation fails, an error is returned.
func NewRepository(canonicalName string) (repo Repository, err error) {
	if len(canonicalName) == 0 {
		return repo, ErrRepositoryNameEmpty
	}
	i := strings.Index(canonicalName, "/")
	if i <= 0 {
		return repo, ErrRepositoryNameMissingHostname
	}
	repo.Hostname = canonicalName[:i]
	repo.Name = canonicalName[i+1:]
	return repo, repo.Validate()
}
