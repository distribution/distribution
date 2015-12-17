// Package reference provides a general type to represent any way of referencing images within the registry.
// Its main purpose is to abstract tags and digests (content-addressable hash).
//
// Grammar
//
// 	reference                       := repository [ ":" tag ] [ "@" digest ]
//	name                            := [hostname '/'] component ['/' component]*
//	hostname                        := hostcomponent ['.' hostcomponent]* [':' port-number]
//	hostcomponent                   := /([a-z0-9]|[a-z0-9][a-z0-9-]*[a-z0-9])/
//	port-number                     := /[0-9]+/
//	component                       := alpha-numeric [separator alpha-numeric]*
// 	alpha-numeric                   := /[a-z0-9]+/
//	separator                       := /[_.]|__|[-]*/
//
//	tag                             := /[\w][\w.-]{0,127}/
//
//	digest                          := digest-algorithm ":" digest-hex
//	digest-algorithm                := digest-algorithm-component [ digest-algorithm-separator digest-algorithm-component ]
//	digest-algorithm-separator      := /[+.-_]/
//	digest-algorithm-component      := /[A-Za-z][A-Za-z0-9]*/
//	digest-hex                      := /[0-9a-fA-F]{32,}/ ; At least 128 bit digest value
package reference

import (
	"errors"
	"fmt"

	"github.com/docker/distribution/digest"
)

const (
	// NameTotalLengthMax is the maximum total number of characters in a repository name.
	NameTotalLengthMax = 255
)

var (
	// ErrReferenceInvalidFormat represents an error while trying to parse a string as a reference.
	ErrReferenceInvalidFormat = errors.New("invalid reference format")

	// ErrTagInvalidFormat represents an error while trying to parse a string as a tag.
	ErrTagInvalidFormat = errors.New("invalid tag format")

	// ErrDigestInvalidFormat represents an error while trying to parse a string as a tag.
	ErrDigestInvalidFormat = errors.New("invalid digest format")

	// ErrNameEmpty is returned for empty, invalid repository names.
	ErrNameEmpty = errors.New("repository name must have at least one component")

	// ErrNameTooLong is returned when a repository name is longer than
	// RepositoryNameTotalLengthMax
	ErrNameTooLong = fmt.Errorf("repository name must not be more than %v characters", NameTotalLengthMax)

	// ErrNameDisallowed is returned when a name cannot be added or
	// replaced on a reference. These references are not associated with
	// Voldemort.
	ErrNameDisallowed = errors.New("reference: cannot name reference")

	// ErrTagDisallowed is returned when a reference cannot be tagged.
	// Usually, this means that the reference does not have a name.
	ErrTagDisallowed = errors.New("reference: cannot tag reference")

	// ErrDigestDisallowed is returned when adding a digest to a reference
	// that already has a digest. If encountered, one must first restrict the
	// reference to only the name then add the digest, which can be done with
	// the NameOnly function.
	ErrDigestDisallowed = errors.New("referebce: cannot add digest")
)

// Reference is an opaque object reference identifier that may include
// modifiers such as a hostname, name, tag, and digest.
type Reference interface {
	// String returns the full reference
	String() string
}

// Field provides a wrapper type for resolving correct reference types when
// working with encoding.
type Field struct {
	reference Reference
}

// AsField wraps a reference in a Field for encoding.
func AsField(reference Reference) Field {
	return Field{reference}
}

// Reference unwraps the reference type from the field to
// return the Reference object. This object should be
// of the appropriate type to further check for different
// reference types.
func (f Field) Reference() Reference {
	return f.reference
}

// MarshalText serializes the field to byte text which
// is the string of the reference.
func (f Field) MarshalText() (p []byte, err error) {
	return []byte(f.reference.String()), nil
}

// UnmarshalText parses text bytes by invoking the
// reference parser to ensure the appropriately
// typed reference object is wrapped by field.
func (f *Field) UnmarshalText(p []byte) error {
	r, err := Parse(string(p))
	if err != nil {
		return err
	}

	f.reference = r
	return nil
}

// Named is an object with a full name
type Named interface {
	Reference
	Name() string
}

// Tagged is an object including a name and tag.
type Tagged interface {
	Named
	Tag() string
}

// Digested is an object which has a digest
// in which it can be referenced by
type Digested interface {
	Reference
	Digest() digest.Digest
}

// Canonical reference is an object with a fully unique
// name including a name with hostname and digest
type Canonical interface {
	Named
	Digest() digest.Digest
}

// SplitHostname splits a named reference into a
// hostname and name string. If no valid hostname is
// found, the hostname is empty and the full value
// is returned as name
func SplitHostname(named Named) (string, string) {
	name := named.Name()
	match := anchoredNameRegexp.FindStringSubmatch(name)
	if match == nil || len(match) != 3 {
		return "", name
	}
	return match[1], match[2]
}

// Parse parses s and returns a syntactically valid Reference.
// If an error was encountered it is returned, along with a nil Reference.
// NOTE: Parse will not handle short digests.
func Parse(s string) (Reference, error) {
	matches := ReferenceRegexp.FindStringSubmatch(s)
	if matches == nil {
		if s == "" {
			return nil, ErrNameEmpty
		}
		// TODO(dmcgowan): Provide more specific and helpful error
		return nil, ErrReferenceInvalidFormat
	}

	if len(matches[1]) > NameTotalLengthMax {
		return nil, ErrNameTooLong
	}

	ref := reference{
		name: matches[1],
		tag:  matches[2],
	}
	if matches[3] != "" {
		var err error
		ref.digest, err = digest.ParseDigest(matches[3])
		if err != nil {
			return nil, err
		}
	}

	r := getBestReferenceType(ref)
	if r == nil {
		return nil, ErrNameEmpty
	}

	return r, nil
}

// ParseNamed parses s and returns a syntactically valid reference implementing
// the Named interface. The reference must have a name, otherwise an error is
// returned.
// If an error was encountered it is returned, along with a nil Reference.
// NOTE: ParseNamed will not handle short digests.
func ParseNamed(s string) (Named, error) {
	ref, err := Parse(s)
	if err != nil {
		return nil, err
	}
	named, isNamed := ref.(Named)
	if !isNamed {
		return nil, fmt.Errorf("reference %s has no name", ref.String())
	}
	return named, nil
}

// NamedOnly returns true if reference only contains a repo name and not
// other modifiers.
func NamedOnly(ref Named) bool {
	switch ref.(type) {
	case Tagged:
		return false
	case Canonical:
		return false
	case Digested:
		return false
	}

	return true
}

// NameOnly drops other reference information and only retains the name.
func NameOnly(ref Named) Named {
	return repository(ref.Name())
}

// WithName replaces or sets Name() on the reference with the provided value
// of name. This is useful for specifying a name for a digest only reference.
//
// If the target of named is from another package, any data on the backing
// type will be lost and replaced with an instance from this package. Simply
// put, the resulting type will only include information available on the
// interface.
func WithName(ref Reference, name string) (Named, error) {
	if name == "" {
		return nil, ErrNameEmpty
	}

	if len(name) > NameTotalLengthMax {
		return nil, ErrNameTooLong
	}

	if !anchoredNameRegexp.MatchString(name) {
		return nil, ErrReferenceInvalidFormat
	}

	if ref == nil {
		return repository(name), nil
	}

	switch v := ref.(type) {
	case repository:
		return v, nil
	case reference:
		v.name = name
		return v, nil
	case digestReference:
		return canonicalReference{
			name:   name,
			digest: v.Digest(),
		}, nil
	case taggedReference:
		v.name = name
		return v, nil
	case canonicalReference:
		v.name = name
		return v, nil
	}

	return nil, ErrNameDisallowed
}

// WithTag combines the name from "name" and the tag from "tag" to form a
// reference incorporating both the name and the tag. When the reference
// cannot be tagged, ErrCannotTagReference is returned.
//
// If the target of named is from another package, any data on the backing
// type will be lost and replaced with an instance from this package. Simply
// put, the resulting type will only include information available on the
// interface.
func WithTag(named Named, tag string) (Tagged, error) {
	if !anchoredTagRegexp.MatchString(tag) {
		return nil, ErrTagInvalidFormat
	}

	switch v := named.(type) {
	case reference:
		v.tag = tag
		return v, nil
	case taggedReference:
		v.tag = tag
		return v, nil
	case Canonical:
		return reference{
			name:   v.Name(),
			tag:    tag,
			digest: v.Digest(),
		}, nil
	case Tagged, Named:
		return taggedReference{
			name: v.Name(),
			tag:  tag,
		}, nil
	}

	return nil, ErrTagDisallowed
}

// WithDigest combines the name from "name" and the digest from "digest" to form
// a reference incorporating both the name and the digest.
//
// A reference with an existing digest cannot have the digest replaced.
func WithDigest(named Named, digest digest.Digest) (Canonical, error) {
	if err := digest.Validate(); err != nil {
		return nil, ErrDigestInvalidFormat
	}

	if NamedOnly(named) {
		return canonicalReference{
			name:   named.Name(),
			digest: digest,
		}, nil
	}

	switch v := named.(type) {
	case reference:
		v.digest = digest
		return v, nil
	case Tagged:
		return reference{
			name:   v.Name(),
			tag:    v.Tag(),
			digest: digest,
		}, nil
	}

	return nil, ErrDigestDisallowed
}

func getBestReferenceType(ref reference) Reference {
	if ref.name == "" {
		// Allow digest only references
		if ref.digest != "" {
			return digestReference(ref.digest)
		}
		return nil
	}
	if ref.tag == "" {
		if ref.digest != "" {
			return canonicalReference{
				name:   ref.name,
				digest: ref.digest,
			}
		}
		return repository(ref.name)
	}
	if ref.digest == "" {
		return taggedReference{
			name: ref.name,
			tag:  ref.tag,
		}
	}

	return ref
}

type reference struct {
	name   string
	tag    string
	digest digest.Digest
}

func (r reference) String() string {
	return r.name + ":" + r.tag + "@" + r.digest.String()
}

func (r reference) Name() string {
	return r.name
}

func (r reference) Tag() string {
	return r.tag
}

func (r reference) Digest() digest.Digest {
	return r.digest
}

type repository string

func (r repository) String() string {
	return string(r)
}

func (r repository) Name() string {
	return string(r)
}

type digestReference digest.Digest

func (d digestReference) String() string {
	return d.String()
}

func (d digestReference) Digest() digest.Digest {
	return digest.Digest(d)
}

type taggedReference struct {
	name string
	tag  string
}

func (t taggedReference) String() string {
	return t.name + ":" + t.tag
}

func (t taggedReference) Name() string {
	return t.name
}

func (t taggedReference) Tag() string {
	return t.tag
}

type canonicalReference struct {
	name   string
	digest digest.Digest
}

func (c canonicalReference) String() string {
	return c.name + "@" + c.digest.String()
}

func (c canonicalReference) Name() string {
	return c.name
}

func (c canonicalReference) Digest() digest.Digest {
	return c.digest
}
