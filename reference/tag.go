package reference

import (
	"fmt"
	"regexp"
)

var (
	// TagRegexp matches valid tag names. From docker/docker:graph/tags.go.
	TagRegexp = regexp.MustCompile(`[\w][\w.-]{0,127}`)

	// TagAnchoredRegexp matches valid tag names, anchored at the start and
	// end of the matched string.
	TagAnchoredRegexp = regexp.MustCompile(`^` + TagRegexp.String() + `$`)

	// ErrTagInvalid is returned when a tag does not match TagAnchoredRegexp.
	ErrTagInvalid = fmt.Errorf("tag name must match %q", TagRegexp.String())
)

// Tag represents an image's tag name.
type Tag string

// NewTag returns a valid Tag from an input string s.
// If the validation fails, an error is returned.
func NewTag(s string) (Tag, error) {
	tag := Tag(s)
	return tag, tag.Validate()
}

// Validate returns ErrTagInvalid if tag does not match TagAnchoredRegexp.
//
//	tag	:= [\w][\w.-]{0,127}
func (tag Tag) Validate() error {
	if !TagAnchoredRegexp.MatchString(string(tag)) {
		return ErrTagInvalid
	}
	return nil
}
