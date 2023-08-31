package reference

import (
	"github.com/distribution/reference"
)

// ParseNormalizedNamed parses a string into a named reference
// transforming a familiar name from Docker UI to a fully
// qualified reference. If the value may be an identifier
// use ParseAnyReference.
//
// Deprecated: use [reference.ParseNormalizedNamed].
func ParseNormalizedNamed(s string) (reference.Named, error) {
	return reference.ParseNormalizedNamed(s)
}

// ParseDockerRef normalizes the image reference following the docker convention,
// which allows for references to contain both a tag and a digest.
//
// Deprecated: use [reference.ParseDockerRef].
func ParseDockerRef(ref string) (reference.Named, error) {
	return reference.ParseDockerRef(ref)
}

// TagNameOnly adds the default tag "latest" to a reference if it only has
// a repo name.
//
// Deprecated: use [reference.TagNameOnly].
func TagNameOnly(ref reference.Named) reference.Named {
	return reference.TagNameOnly(ref)
}

// ParseAnyReference parses a reference string as a possible identifier,
// full digest, or familiar name.
//
// Deprecated: use [reference.ParseAnyReference].
func ParseAnyReference(ref string) (reference.Reference, error) {
	return reference.ParseAnyReference(ref)
}
