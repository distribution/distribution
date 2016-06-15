package reference

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/distribution/digest"
)

var (
	legacyDefaultDomain = "index.docker.io"
	defaultDomain       = "docker.io"
	defaultRepoPrefix   = "library/"
	defaultTag          = "latest"
)

// NormalizedName parses a string into a named reference
// transforming a familiar name from Docker UI to a fully
// qualified reference. If the value may be an identifier
// use ParseAnyReference.
func NormalizedName(s string) (Named, error) {
	if ok := anchoredIdentifierRegexp.MatchString(s); ok {
		return nil, fmt.Errorf("invalid repository name (%s), cannot specify 64-byte hexadecimal strings", s)
	}
	domain, remainder := splitDockerDomain(s)
	var remoteName string
	if tagSep := strings.IndexRune(remainder, ':'); tagSep > -1 {
		remoteName = remainder[:tagSep]
	} else {
		remoteName = remainder
	}
	if strings.ToLower(remoteName) != remoteName {
		return nil, errors.New("invalid reference format: repository name must be lowercase")
	}

	return ParseNamed(domain + "/" + remainder)
}

// splitDockerDomain splits a repository name to domain and remotename string.
// If no valid domain is found, the default domain is used. Repository name
// needs to be already validated before.
func splitDockerDomain(name string) (domain, remainder string) {
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ".:") && name[:i] != "localhost") {
		domain, remainder = defaultDomain, name
	} else {
		domain, remainder = name[:i], name[i+1:]
	}
	if domain == legacyDefaultDomain {
		domain = defaultDomain
	}
	if domain == defaultDomain && !strings.ContainsRune(remainder, '/') {
		remainder = defaultRepoPrefix + remainder
	}
	return
}

// FamiliarName returns a shortened version of the name familiar
// to to the Docker UI. Familiar names have the default domain
// "docker.io" and "library/" repository prefix removed.
// For example, "docker.io/library/redis" will have the familiar
// name "redis" and "docker.io/dmcgowan/myapp" will be "dmcgowan/myapp".
func FamiliarName(named Named) Named {
	var repo repository
	repo.domain, repo.path = splitDomain(named.Name())

	if repo.domain == defaultDomain {
		repo.domain = ""
		repo.path = strings.TrimPrefix(repo.path, defaultRepoPrefix)
	}
	if digested, ok := named.(Digested); ok {
		return canonicalReference{
			repository: repo,
			digest:     digested.Digest(),
		}
	}
	if tagged, ok := named.(Tagged); ok {
		return taggedReference{
			repository: repo,
			tag:        tagged.Tag(),
		}
	}
	return repo
}

// EnsureTagged adds the default tag "latest" to a reference if it only has
// a repo name.
func EnsureTagged(ref Named) NamedTagged {
	namedTagged, ok := ref.(NamedTagged)
	if !ok {
		namedTagged, err := WithTag(ref, defaultTag)
		if err != nil {
			// Default tag must be valid, to create a NamedTagged
			// type with non-validated input the WithTag function
			// should be used instead
			panic(err)
		}
		return namedTagged
	}
	return namedTagged
}

// ParseAnyReference parses a reference string as a possible identifier,
// full digest, or familiar name.
func ParseAnyReference(ref string) (Reference, error) {
	if ok := anchoredIdentifierRegexp.MatchString(ref); ok {
		return digestReference("sha256:" + ref), nil
	}
	if dgst, err := digest.ParseDigest(ref); err == nil {
		return digestReference(dgst), nil
	}

	return NormalizedName(ref)
}

// ParseAnyReferenceWithSet parses a reference string as a possible short
// identifier to be matched in a digest set, a full digest, or familiar name.
func ParseAnyReferenceWithSet(ref string, ds *digest.Set) (Reference, error) {
	if ok := anchoredShortIdentifierRegexp.MatchString(ref); ok {
		dgst, err := ds.Lookup(ref)
		if err == nil {
			return digestReference(dgst), nil
		}
	} else {
		if dgst, err := digest.ParseDigest(ref); err == nil {
			return digestReference(dgst), nil
		}
	}

	return NormalizedName(ref)
}
