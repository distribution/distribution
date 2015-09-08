package reference

import "regexp"

var (
	// nameComponentRegexp restricts registry path component names to
	// start with at least one letter or number, with following parts able to
	// be separated by one period, dash or underscore.
	nameComponentRegexp = regexp.MustCompile(`[a-zA-Z0-9]+(?:[._-][a-z0-9]+)*`)

	nameRegexp = regexp.MustCompile(`(?:` + nameComponentRegexp.String() + `/)*` + nameComponentRegexp.String())

	hostnameComponentRegexp = regexp.MustCompile(`(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])`)

	// hostnameComponentRegexp restricts the registry hostname component of a repository name to
	// start with a component as defined by hostnameRegexp and followed by an optional port.
	hostnameRegexp = regexp.MustCompile(`(?:` + hostnameComponentRegexp.String() + `\.)*` + hostnameComponentRegexp.String() + `(?::[0-9]+)?`)

	// TagRegexp matches valid tag names. From docker/docker:graph/tags.go.
	TagRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

	// anchoredTagRegexp matches valid tag names, anchored at the start and
	// end of the matched string.
	anchoredTagRegexp = regexp.MustCompile(`^` + TagRegexp.String() + `$`)

	// NameRegexp is the format for the name component of references. The
	// regexp has capturing groups for the hostname and name part omitting
	// the seperating forward slash from either.
	NameRegexp = regexp.MustCompile(`(?:` + hostnameRegexp.String() + `/)?` + nameRegexp.String())

	// ReferenceRegexp is the full supported format of a reference. The
	// regexp has capturing groups for name, tag, and digest components.
	ReferenceRegexp = regexp.MustCompile(`^((?:` + hostnameRegexp.String() + `/)?` + nameRegexp.String() + `)(?:[:](` + TagRegexp.String() + `))?(?:[@]([A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}))?$`)

	// anchoredNameRegexp is used to parse a name value, capturing hostname
	anchoredNameRegexp = regexp.MustCompile(`^(?:(` + hostnameRegexp.String() + `)/)?(` + nameRegexp.String() + `)$`)
)
