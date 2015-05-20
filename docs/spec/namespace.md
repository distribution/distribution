# Namespace
The global namespace represents the full set of named repositories which can be
referenced by Docker. Any scope within the namespace may refer to a subset of
the named repos. A repository contains a set of content-addressable blobs and
tags referencing those blobs. The namespace can be used to discover the
location and certificate information of a repository based on DNS, HTTP
requests and other means.

A repository should always be referenced by its fully qualified name. If a
client presents a shortened name to a user, that name should be fully expanded
based on rules defined by the client before contacting a remote server. When
mirroring repositories, the original name for a repository must be used.
Changing a repository name is equivalent to copying or moving and will not be
referenced by the original repository.

## Terminology

 - *Global Namespace* - The full set of referenceable names.
 - *Name* - A fully qualified string containing both the domain and resource name
 - *Repository* - A collection of objects under the same name within the
namespace.
 - *Namespace* - also *Namespace Scope* - A collection of repositories with a
common name prefix and set of services including registry API, index, and trust
context.
 - *Short Name* - A name which does not contain a domain and requires
expansion to a fully qualified name before resolving to a repository.

## Format
A name consists of two parts, a DNS host name plus a repository path. The host
name follows the DNS rules without change. The total length of a name is 255
characters however there is no specific limitation on DNS or path components.

### Name Grammar
```
<name> ::= <hostname>"/"<path>
<hostname> ::= <host-part>*["."<host-part>][":"<port>]
<path> ::= <path-part>*["/"<path-part>]
<host-part> ::= <regexp "[a-z]([-]?[a-z0-9])*">
<port> ::= <number 1 to 65535>
<path-part> ::= <regexp "[a-z0-9]([._-]?[a-z0-9])*">
```

## Metadata
The metadata for a namespace is a list of entries consisting of a scope, action,
and space separate arguments. Each action may interpret the arguments
differently. The scope of an individual entry defines which namespaces the
value may apply. It is up to the resolution process to return the set of
metadata which should be applied for a given name, however any returned values
which are out of scope should be considered invalid.

### Actions
#### pull

Used to represent a registry endpoint which supports pull operations. This
may include full registries as well as read-only mirrors.

The arguments for pull consist of a registry endpoint as well as optional
arguments for priority and key=value flags.

`<registry endpoint> [<priority>] [<flag>[=<value>], ...]`

##### Priority

Integer value providing relative sort order between other endpoints with the
same action. Higher priority endpoints should be tried before lower priority
endpoints.

##### Flags

| Key | Value | Default | Description |
|---|---|---|---|
| trim | boolean | false | Whether this registry endpoint expects the hostname to be trimmed from the API requests. This is used for compatibility with existing registries |
| version | string | "2.0" | Which API version the registry implements |
| notag | boolean | false | Whether tag operations are not supported by this registry |

#### push

Used to represent a registry endpoint which supports push operations. Should
never be defined for read-only mirrors.

Push uses the same arguments as pull.

#### index

Used to represent a search index endpoint.

`<registry endpoint> [version=<value>]`

#### namespace

Used to extend the interface to a parent or stop further namespace processing
by not providing any arguments. When a parent is provided as an argument,
namespace processing should continue by including the resolved values of the
the parent. A namespace action without a scope can be used to turn off a
namespace by providing no values except a namespace action. This will end
processing since a value is found however not provide any metadata which can
be used to configure the endpoint.

`[<parent scope>, ...]`

## Discovery
The discovery process involves resolving a name into namespace scoped metadata.
The namespace metadata contains the full set of information needed to fetch and
verify content associated with the namespace repository. The metadata includes
list of registry API endpoints, the trust model, and search index. The discovery
process should not be considered secure and therefore certificates retrieved as
part of the discovery process should be verified before trusting.

Discovery can be defined as...
`<fully qualified name> -> scope([<registry API endpoint>, ...], <publisher certificate>, <search index>, ...)`

or in Go as...
```go
type Resolver interface {
	Resolve(name string) Metadata
}
```

### Default Method

The first element of the namespace is extracted and used as the domain name for
resolution. The domain name should be used unmodified.

`<fully qualified name> -> <domain>/<path>`

#### HTTPS
The discovery related metadata will be fetched via HTTPS from the DNS resolved
location using the remaining namespace path elements as the HTTP path in a GET
request. A discovery request URL would be in the format
`https://<domain>/<name>?docker-discovery=1`

For example, "example.com/foo/bar" would create a URL
"https://example.com/foo/bar?docker-discovery=1"

##### HTML Body
```html
<meta name="docker-scope" content="example.com"><!-- Applies to all metadata -->
<meta name="docker-registry-push" content="https://registry.example.com/v2/ version=2.0 trim">
<meta name="docker-registry" content="https://registry.example.com/v1/ version=1.0">
<meta name="docker-registry-pull" content="https://registry.mirror.com/v2/ version=2.0">
<meta name="docker-registry-pull" content="http://registry.mirror.com/v2/ version=2.0">
<meta name="docker-index" content="https://search.mirror.com/v1/ version=1.0">
```

| Name | Action | Content |
|---|---|---|
| docker-scope | | fully qualified name |
| docker-registry | push+pull | pull arguments |
| docker-registry-pull | pull | pull arguments |
| docker-registry-push | push | push arguments |
| docker-index | index | index arguments |
| docker-namespace | namespace | fully qualified name |

#### Fallback (Compatibility)
If HTTPS is not implemented for a namespace, a fallback protocol may be used.
The fallback process involves attempting to ping possible registry API
endpoints to determine the set of endpoints and using no trust model. This
should preferably be used only when a namespace is explicitly marked as
insecure.

### Extensibility
A custom method may be used to provide discovery by implementing the
`Resolver` interface.

### Scope
The namespace information produced from the discovery process may contain a
scope field. The scope field means the information may apply to any namespace
with a prefix of the given scope. The scope prefix will always be applied with
a path separator. If the scope field is omitted, the information may not be
applied to any other namespace.

### Endpoints
The registry API endpoints each contain a version (may be v1 or v2 registries)
and may either be pull only mirrors or full registries. The trust model applies
to each registry API endpoint and may not be overloaded by an individual
endpoint. The trust model defines the method for verifying the content
retrieved from an endpoint, not the method for authentication or authorization.
Each registry API endpoint is responsible for specifying its authentication or
authorization method.

It may also be possible in the future to extend these endpoints to support
direct downloads of tarballs, such as the result from a `docker save`.

## Name Expansion
Before a name can be resolved, it must be expanded to its fully qualified form.
This may mean adding to the resource path as well as inserting a default
domain. The rules for expansion must be determined by the client.

Expansion can be defined as...
`<name> -> <fully qualified name>`

### Compatibility
Current Docker clients have a default expansion which must remain backwards
compatible from a user perspective. Docker clients expand short names containing
no slashes as "docker.io/library/{name}" and all other short names as
"docker.io/{name}". Current tooling built around Docker expects to use the
Docker hub registry for all short names.

