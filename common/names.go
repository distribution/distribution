package common

import (
	"regexp"
)

// RepositoryNameComponentRegexp restricts registtry path components names to
// start with at least two letters or numbers, with following parts able to
// separated by one period, dash or underscore.
var RepositoryNameComponentRegexp = regexp.MustCompile(`[a-z0-9]{2,}(?:[._-][a-z0-9]+)*`)

// TODO(stevvooe): RepositoryName needs to be limited to some fixed length.
// Looking path prefixes and s3 limitation of 1024, this should likely be
// around 512 bytes. 256 bytes might be more manageable.

// RepositoryNameRegexp builds on RepositoryNameComponentRegexp to allow 2 to
// 5 path components, separated by a forward slash.
var RepositoryNameRegexp = regexp.MustCompile(`(?:` + RepositoryNameComponentRegexp.String() + `/){1,4}` + RepositoryNameComponentRegexp.String())

// TagNameRegexp matches valid tag names. From docker/docker:graph/tags.go.
var TagNameRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

// TODO(stevvooe): Contribute these exports back to core, so they are shared.
