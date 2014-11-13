package common

import (
	"regexp"
)

// RepositoryNameComponentRegexp restricts registtry path components names to
// start with at least two letters or numbers, with following parts able to
// separated by one period, dash or underscore.
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-z0-9]{2,}(?:[._-][a-z0-9]+)*`)

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow 2 to
// 5 path components, separated by a forward slash.
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameComponentRegexp.String() + `/){1,4}` + RepositoryNameComponentRegexp.String())

// TagNameRegexp matches valid tag names. From docker/docker:graph/tags.go.
var TagNameRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

// TODO(stevvooe): Contribute these exports back to core, so they are shared.
