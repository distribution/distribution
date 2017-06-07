# Namespace
The namespace represents the full set of named repositories which can be
referenced by Docker. A repository contains a set of content-addressable blobs
and tags referencing those blobs. The namespace can be used to discover the
location and trust information of a repository based on DNS, HTTP requests and
other means. The namespace is global and owned through the domain at the root of
the namespace path.

A repository should always be referenced by its fully canonical value. If a
client presents a shortened name to a user, that name should be fully
canonicalized based on rules defined by the client before contacting a remote
server. When mirroring repositories, the original name for a repository must be
used. Changing a repository name is equivalent to copying or moving and will not
be referenced by the original repository.

## Format
A name consists of two parts, a DNS host name plus a repository path. The host
name follows the DNS rules without change. The total length of a name is 255
characters however there is no specific limitation on DNS or path components.

### Name Grammar
~~~
<name> ::= <hostname>"/"<path>
<hostname> ::= <host-part>*["."<host-part>][":"<port>]
<path> ::= <path-part>*["/"<path-part>]
<host-part> ::= <regexp "[a-z]([-]?[a-z0-9])*">
<port> ::= <number 1 to 65535>
<path-part> ::= <regexp "[a-z0-9]([._-]?[a-z0-9])*">
~~~

## Discovery
The discovery process involves resolving a namespace into registry metadata. The
registry metadata contains the full set of information needed to fetch and
verify content associated with the namespace repository. The metadata includes
list of registry endpoints, the trust model, and search index.

Discovery can be defined as...
`<namespace> -> ([<endpoint>, ...], <trust info>, <search index>)`

or in Go as...
~~~go
type RegistryResolver interface {
	Resolve(name string) Registry
}
~~~

### Default Method

The first element of the namespace is extracted and used as the domain name for
resolution. The domain name should be used unmodified.

`<namespace> -> <domain>/<name>`

#### HTTPS
The discovery related metadata will be fetched via HTTPS from the DNS resolved
location using the remaining namespace path elements as the HTTP path in a GET
request. A discovery request url would be in the format
https://<domain>/<name>?docker-discovery=1

For example, “example.com/foo/bar” would create a url
“https://example.com/foo/bar?docker-discovery=1”

##### HTML Response
~~~html
<meta name="docker-scope" content="registry.example.com">
<meta name=“docker-registry” content=“push,pull v2 https://registry.example.com/v2/”>
<meta name=“docker-registry” content=“push,pull v1 https://registry.example.com/v1/”>
<meta name=“docker-registry” content=“pull v2 https://registry.mirror.com/v2/”>
<meta name=“docker-registry” content=“pull,notag v2 http://registry.mirror.com/v2/”>
<meta name=“docker-search” content=“v1 https://search.mirror.com/v1/”>
<meta name=“docker-trust” content=“tuf https://registry.example.com/{name}/}”>
~~~

#### Fallback (Compatibility)
If HTTPS is not implemented for a namespace, a fallback protocol may be
used when the namespace root domain is explicitly marked as insecure. The
fallback process involves attempting to ping possible registry endpoints to
determine the set of endpoints and using no trust model.

### Extensibility
A custom method may be used to provide discovery by implementing the
`RegistryResolver` interface.

### Scope
The registry information produced from the discovery process may contain a scope
field. The scope field means the information may apply to any namespace with a
prefix of the given scope. The Scope prefix will always be applied with a path
separator. If the scope field is ommitted, the information may not be applied to
any other namespace.

### Endpoints
The registry endpoints each contain a version (may be v1 or v2 registries) and
may either be pull only mirrors or full registries. The trust model applies to
each registry endpoint and may not be overloaded by an individual endpoint. The
trust model defines the method for verifying the content retrieved from an
endpoint, not the method for authentication or authorization.  Each registry
endpoint is responsible for specifying its authentication or authorization
method. 

It may also be possible in the future to extend these endpoints to support
direct downloads of tarballs, such as the result from a `docker save`.

## Trust Model
The trust model used by a registry is responsible for empowering the client to
verify content within the namespace without trusting the individual endpoints
providing content, which may or may not be part of the same entity which owns
the namespace. The trust model enables scalable mirroring of content as well as
peer-to-peer distribution.

The trust model is primarily responsible for providing verified tuples of
(namespace, tag, content-address). The content-address may point directly at a
image manifest which can itself by verified by its hash as well as provide 
additional content-addresses.

### The Update Framework
TUF would be used to get a trusted list of tag to manifest content addresses.
Once this list has been verified as legitimate and up-to-date, the client would
contact the registry for specific manifest content-addresses. The tag feature of
the registry would not be used.

Target files contain a content address, content-type, and size. There will be
target files for individual tags. The content-address will point at a manifest
type in the registry.

