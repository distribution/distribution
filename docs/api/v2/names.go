package v2

import (
	"fmt"
	"regexp"
	"strings"
)

// TODO(stevvooe): Move these definitions back to an exported package. While
// they are used with v2 definitions, their relevance expands beyond.
// "distribution/names" is a candidate package.

const (
	// RepositoryNameComponentMinLength is the minimum number of characters in a
	// single repository name slash-delimited component
	RepositoryNameComponentMinLength = 2

	// RepositoryNameComponentMaxLength is the maximum number of characters in a
	// single repository name slash-delimited component
	RepositoryNameComponentMaxLength = 30

	// RepositoryNameMinComponents is the minimum number of slash-delimited
	// components that a repository name must have
	RepositoryNameMinComponents = 1

	// RepositoryNameMaxComponents is the maximum number of slash-delimited
	// components that a repository name must have
	RepositoryNameMaxComponents = 5

	// RepositoryNameTotalLengthMax is the maximum total number of characters in
	// a repository name
	RepositoryNameTotalLengthMax = 255
)

// RepositoryNameComponentRegexp restricts registtry path components names to
// start with at least two letters or numbers, with following parts able to
// separated by one period, dash or underscore.
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-z0-9]+(?:[._-][a-z0-9]+)*`)

// RepositoryNameComponentAnchoredRegexp is the version of
// RepositoryNameComponentRegexp which must completely match the content
var RepositoryNameComponentAnchoredRegexp = regexp.MustCompile(`^` + RepositoryNameComponentRegexp.String() + `$`)

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow 1 to
// 5 path components, separated by a forward slash.
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameComponentRegexp.String() + `/){0,4}` + RepositoryNameComponentRegexp.String())

// TagNameRegexp matches valid tag names. From docker/docker:graph/tags.go.
var TagNameRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

// TODO(stevvooe): Contribute these exports back to core, so they are shared.

var (
	// ErrRepositoryNameComponentShort is returned when a repository name
	// contains a component which is shorter than
	// RepositoryNameComponentMinLength
	ErrRepositoryNameComponentShort = fmt.Errorf("respository name component must be %v or more characters", RepositoryNameComponentMinLength)

	// ErrRepositoryNameComponentLong is returned when a repository name
	// contains a component which is longer than
	// RepositoryNameComponentMaxLength
	ErrRepositoryNameComponentLong = fmt.Errorf("respository name component must be %v characters or less", RepositoryNameComponentMaxLength)

	// ErrRepositoryNameMissingComponents is returned when a repository name
	// contains fewer than RepositoryNameMinComponents components
	ErrRepositoryNameMissingComponents = fmt.Errorf("repository name must have at least %v components", RepositoryNameMinComponents)

	// ErrRepositoryNameTooManyComponents is returned when a repository name
	// contains more than RepositoryNameMaxComponents components
	ErrRepositoryNameTooManyComponents = fmt.Errorf("repository name %v or less components", RepositoryNameMaxComponents)

	// ErrRepositoryNameLong is returned when a repository name is longer than
	// RepositoryNameTotalLengthMax
	ErrRepositoryNameLong = fmt.Errorf("repository name must not be more than %v characters", RepositoryNameTotalLengthMax)

	// ErrRepositoryNameComponentInvalid is returned when a repository name does
	// not match RepositoryNameComponentRegexp
	ErrRepositoryNameComponentInvalid = fmt.Errorf("repository name component must match %q", RepositoryNameComponentRegexp.String())
)

// ValidateRespositoryName ensures the repository name is valid for use in the
// registry. This function accepts a superset of what might be accepted by
// docker core or docker hub. If the name does not pass validation, an error,
// describing the conditions, is returned.
func ValidateRespositoryName(name string) error {
	if len(name) > RepositoryNameTotalLengthMax {
		return ErrRepositoryNameLong
	}

	components := strings.Split(name, "/")

	if len(components) < RepositoryNameMinComponents {
		return ErrRepositoryNameMissingComponents
	}

	if len(components) > RepositoryNameMaxComponents {
		return ErrRepositoryNameTooManyComponents
	}

	for _, component := range components {
		if len(component) < RepositoryNameComponentMinLength {
			return ErrRepositoryNameComponentShort
		}

		if len(component) > RepositoryNameComponentMaxLength {
			return ErrRepositoryNameComponentLong
		}

		if !RepositoryNameComponentAnchoredRegexp.MatchString(component) {
			return ErrRepositoryNameComponentInvalid
		}
	}

	return nil
}
