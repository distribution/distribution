// Package reference provides a general type to represent any way of referencing images within the registry.
// Its main purpose is to abstract tags and digests (content-addressable hash).
//
// Grammar
//
// 	reference                       := repository [ ":" tag ] [ "@" digest ]
//
//	// repository.go
//	repository			:= hostname ['/' component]+
//	hostname 			:= component [':' port-number]
//	component			:= alpha-numeric [separator alpha-numeric]*
// 	alpha-numeric			:= /[a-zA-Z0-9]+/
//	separator			:= /[._-]/
//	port-number			:= /[0-9]+/
//
//	// tag.go
//	tag                             := /[\w][\w.-]{0,127}/
//
//	// from the digest package
//	digest                          := digest-algorithm ":" digest-hex
//	digest-algorithm                := digest-algorithm-component [ digest-algorithm-separator digest-algorithm-component ]
//	digest-algorithm-separator      := /[+.-_]/
//	digest-algorithm-component      := /[A-Za-z]/ /[A-Za-z0-9]*/
//	digest-hex                      := /[A-Za-z0-9_-]+/ ; supports hex bytes or url safe base64
package reference

import (
	"errors"
	"regexp"

	"github.com/docker/distribution/digest"
)

// ErrReferenceInvalidFormat represents an error while trying to parse a string as a reference.
var ErrReferenceInvalidFormat = errors.New("invalid reference format")

// Reference abstracts types that reference images in a certain way.
type Reference interface {
	// Repository returns the repository part of a reference
	Repository() Repository
	// String returns the entire reference, including the repository part
	String() string
}

func parseHostname(s string) (hostname, tail string) {
	tail = s
	i := regexp.MustCompile(`^` + RepositoryNameHostnameRegexp.String()).FindStringIndex(s)
	if i == nil {
		return
	}
	return s[:i[1]], s[i[1]:]
}

func parseRepositoryName(s string) (repo, tail string) {
	tail = s
	i := regexp.MustCompile(`^/(?:` + RepositoryNameComponentRegexp.String() + `/)*` + RepositoryNameComponentRegexp.String()).FindStringIndex(s)
	if i == nil {
		return
	}
	return s[:i[1]], s[i[1]:]
}

func parseTag(s string) (tag Tag, tail string) {
	tail = s
	if len(s) == 0 || s[0] != ':' {
		return
	}
	tag, err := NewTag(s[1:])
	if err != nil {
		return
	}
	tail = s[len(tag)+1:]
	return
}

func parseDigest(s string) (dgst digest.Digest, tail string) {
	tail = s
	if len(s) == 0 || s[0] != '@' {
		return
	}
	dgst, err := digest.ParseDigest(s[1:])
	if err != nil {
		return
	}
	tail = s[len(dgst)+1:]
	return
}

// Parse parses s and returns a syntactically valid Reference.
// If an error was encountered it is returned, along with a nil Reference.
func Parse(s string) (Reference, error) {
	hostname, s := parseHostname(s)
	name, s := parseRepositoryName(s)
	repository := Repository{Hostname: hostname, Name: name}
	if err := repository.Validate(); err != nil {
		return nil, err
	}
	tag, s := parseTag(s)
	dgst, s := parseDigest(s)
	if len(s) > 0 {
		return nil, ErrReferenceInvalidFormat
	}

	if dgst != "" {
		return DigestReference{repository: repository, digest: dgst, tag: tag}, nil
	}
	if tag != "" {
		return TagReference{repository: repository, tag: tag}, nil
	}
	return nil, ErrReferenceInvalidFormat
}

// DigestReference represents a reference of the form `repository@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef`.
// Implements the Reference interface.
type DigestReference struct {
	repository Repository
	digest     digest.Digest
	tag        Tag
}

// Repository returns the repository part.
func (r DigestReference) Repository() Repository { return r.repository }

// String returns the full string reference.
func (r DigestReference) String() string {
	return r.repository.String() + "@" + string(r.digest)
}

// NewDigestReference returns an initialized DigestReference.
func NewDigestReference(canonicalRepository string, digest digest.Digest, optionalTag Tag) (DigestReference, error) {
	ref := DigestReference{}

	repo, err := NewRepository(canonicalRepository)
	if err != nil {
		return ref, err
	}
	ref.repository = repo

	if err := digest.Validate(); err != nil {
		return ref, err
	}
	ref.digest = digest

	if len(optionalTag) > 0 {
		if err := optionalTag.Validate(); err != nil {
			return ref, err
		}
		ref.tag = optionalTag
	}

	return ref, err
}

// TagReference represents a reference of the form `repository:tag`.
// Implements the Reference interface.
type TagReference struct {
	repository Repository
	tag        Tag
}

// Repository returns the repository part.
func (r TagReference) Repository() Repository { return r.repository }

// String returns the full string reference.
func (r TagReference) String() string {
	return r.repository.String() + ":" + string(r.tag)
}

// NewTagReference returns an initialized TagReference.
func NewTagReference(canonicalRepository string, tagName string) (TagReference, error) {
	ref := TagReference{}

	repo, err := NewRepository(canonicalRepository)
	if err != nil {
		return ref, err
	}
	ref.repository = repo

	tag, err := NewTag(tagName)
	if err != nil {
		return ref, err
	}
	ref.tag = tag

	return ref, err
}
