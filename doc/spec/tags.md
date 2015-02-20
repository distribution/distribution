# Tags and Signatures

## Introduction

Classically, the role of tagging has been less formal in the docker eco-
system. Effectively, the concept of "tagging" meant that one assigned a name
to an image id.

Tags in docker are missing several features that one might expect when coming
from other systems. In git, one can tag any commit and then sign that tag,
providing a name to a given revision and the ability to verify that the name
was assigned by a trusted party. We'd like bring this more familar model to
the docker ecosystem.

The following changes are proposed:

1. Tags will become first-class object in the image distribution data model.
2. Tags are the primary mechanism for verification and named provenance.

The tag object's role will be equivalent to the v2, schema version 1.

## Tags

A tag is simply a named pointer to content. The content can be any blob but
should mostly be a manifest. One can sign tags to later verify that they were
created by a trusted party.

This also relies on signatures being a first-class component. We may explore
that in a separate proposal or integrate signatures completely with tags.

### Routes

The following routes are added to the V2 API specification:

|Method|Path|Entity|Description|
-------|----|------|------------
| GET    | `/v2/<name>/tags/`         | Tag | List the tags for the repo. Must end with slash.      |
| GET    | `/v2/<name>/tags/<tag>`    | Tag | Return the tag object identified by `name` and `tag`. |
| PUT    | `/v2/<name>/tags/<tag>`    | Tag | Put the tag identified by `name` and `tag`.           |
| DELETE | `/v2/<name>/tags/<tag>`    | Tag | Delete the tag identified by `name` and `tag`.        |

Note that for compatibility with the first version 2 specification, requests
to "/v2/<name>/tags/<tag>" will require an accept header with the value of
"application/vnd.docker.distribution.tag.v1+json" to disambiguate from the
endpoint "/v2/<name>/tags/list".

### Media Types

Tag's may have the following media types:

| Media Type                                             | Description                                      |
---------------------------------------------------------|--------------------------------------------------|
| `application/vnd.docker.distribution.tag.v1+json`      | Base tag object                                  |
| `application/vnd.docker.distribution.tag.v1+jws`       | tag object wrapped in one or more jws signatures |
| `application/vnd.docker.distribution.tag.v1+prettyjws` | tag object with jws signatures in pretty format  |
| `application/vnd.docker.distribution.tags.v1+json`     | List of tag objects.                             |

### Fields

The fields of a tag object are described, as follows:

<dl>
	<dt>name</dt>
	<dd>
		The fully qualified name and tag in docker format, exactly as it would
		appear on the docker command line. For an image named
		"example.com/foo/bar" with tag "latest", the value would be
		"example.com/foo/bar:latest".
	</dd>

	<dt>description</dt>
	<dd>A short, human-readable description of the tag and its target.</dd>

	<dt>meta</dt>
	<dd>Opaque, user-defined fields, keyed by strings. There are no limits to the structure.</dd>

	<dt>target</dt>
	<dd>
		<dl>
			<dt>mediatype</dt>
			<dd>The media type of the target object.</dd>

			<dt>size</dt>
			<dd>
				Size, in bytes, of the targeted content.
			</dd>

			<dt>digest</dt>
			<dd>
				A digest that identifies the target object. The first entry
				should be considered canonical.
			</dd>
		</dl>
	</dd>
</dl>

#### Example

The following is an example of a tag object, with media type
`application/vnd.docker.distribution.tag.v1+json`:

```json
{
	"name": "example.com/foo/bar:latest",
	"description": "A short description about this tag",
	"meta": {
		"repo": "github.com/docker/distribution",
		"commit": "gabcdef",
	},
	"target": {
		"mediatype": "application/vnd.docker.distribution.manifest.v1+json",
		"digest": "sha256:...",
		"size": 1024
	}
}
```

If the tag is requested with media type
`application/vnd.docker.distribution.tag.v1+prettyjws`, signatures for the tag
will be included with the request. This is similar to how V1, schemaVersion 1
manifests are currently served. An example follows:


```json
{
	"name": "example.com/foo/bar:latest",
	"description": "A short description about this tag",
	"meta": {
		"repo": "github.com/docker/distribution",
		"commit": "gabcdef",
	},
	"target": {
		"mediatype": "application/vnd.docker.distribution.manifest.v1+json",
		"digest": "sha256:...",
		"size": 1024
	},
	"signatures": ...
}
```

A regular JWS will be served up with the media type
`application/vnd.docker.distribution.tag.v1+jws`

### Equivalence and Aliases

If two tags point at an identical target, they are considered to be
"equivalent tags" or "aliases". For the purposes of this proposal, tag
equivalence will be interpreted on the client-side and no endpoints will be
provided to identify sets of equivalent tags.

Other proposals have championed the notion of an "aliases" field on a manifest
or tag object to specify alternate names. That approach has been avoided in
favor of writing a tag for each alias in this proposal. Opting for bespoke
tags allows us to support convergent yet verifiable aliases and avoids
indexing manifest content on the registry.

### Compatibility

The new tag object is roughly equivalent to the current manifests in that it
is a named reference. The main difference is that it points to a single
object, rather than several layers. Manifests continue to be pushed and pulled
in the same manner as before. However, if the new manifest type is requested,
it's tag information will not be included. If a new manifest is pulled but the
old format is requested, the manifest and signatures must be assembled from
tags and existing manifest.

## Road Map

1. Garner feedback on this proposal.
2. Add new V2 API routes to read and manipulate tags to specification.
3. Integrate support into registry API and docker engine.