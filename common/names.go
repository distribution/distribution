package common

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	RepositoryNameComponentMinLength = 2
	RepositoryNameComponentMaxLength = 30

	RepositoryNameMinComponents  = 2
	RepositoryNameMaxComponents  = 5
	RepositoryNameTotalLengthMax = 255
)

// RepositoryNameComponentRegexp restricts registtry path components names to
// start with at least two letters or numbers, with following parts able to
// separated by one period, dash or underscore.
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-z0-9]+(?:[._-][a-z0-9]+)*`)
var RepositoryNameComponentAnchoredRegexp = regexp.MustCompile(`^` + RepositoryNameComponentRegexp.String() + `$`)

// TODO(stevvooe): RepositoryName needs to be limited to some fixed length.
// Looking path prefixes and s3 limitation of 1024, this should likely be
// around 512 bytes. 256 bytes might be more manageable.

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow 2 to
// 5 path components, separated by a forward slash.
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameComponentRegexp.String() + `/){1,4}` + RepositoryNameComponentRegexp.String())

// TagNameRegexp matches valid tag names. From docker/docker:graph/tags.go.
var TagNameRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

// TODO(stevvooe): Contribute these exports back to core, so they are shared.

var (
	ErrRepositoryNameComponentShort = fmt.Errorf("respository name component must be %v or more characters", RepositoryNameComponentMinLength)
	ErrRepositoryNameComponentLong  = fmt.Errorf("respository name component must be %v characters or less", RepositoryNameComponentMaxLength)

	ErrRepositoryNameMissingComponents = fmt.Errorf("repository name must have at least %v components", RepositoryNameMinComponents)
	ErrRepositoryNameTooManyComponents = fmt.Errorf("repository name %v or less components", RepositoryNameMaxComponents)

	ErrRepositoryNameLong             = fmt.Errorf("repository name must not be more than %v characters", RepositoryNameTotalLengthMax)
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
