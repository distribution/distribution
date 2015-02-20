// Package distribution defines the interfaces for components of docker
// distribution. The goal is to allow users to reliably package, ship and
// store content related to docker images.
//
// This is accomplished with a set of objects that are used to describe
// runnable docker images. The relationships between the objects instruct
// consumers on how to assemble an image.
//
// Blob
//
// The central abstraction is a Blob. All objects can be accessed, created and
// written to a blob store. Higher-level interfaces are responsible for
// working with application objects central to the distribution model. As a
// rule, all objects are responsible for describing their own serialization
// and media type.
//
// Blobs are identified with a cryptographically strong hash, referred to as
// the digest. The digest is combined with the mediatype in Descriptor to
// provide a detached handle that can be used to compare and identify blob
// content.
//
// Manifest
//
// A manifest is a blob that depends on other blobs. Effectively, the
// repository describes trees of interrelated objects. The manifest is the
// branch node for such a tree.
//
// In docker, the manifest is the primary abstraction of a runnable image. The
// dependencies describe the layers of an image and opaque application data
// instructs a docker engine how to run it.
//
// Tag
//
// A tag provides a unique name for a particular digest. If a manifest is a
// tree node that can point many objects, a tag maps a human readable name to
// a single object. Tags are the primary mechanism for naming manifests within
// docker. When one runs an image named "library/ubuntu:latest", the target
// manifest would be resolved by the tag.
//
// Signatures
//
// A signature is a blob of data associated with a digest. One or more
// signatures can be associated with any blob. The interpretation of the
// signature is delegated to the external trust system. A BlobStore may add
// signatures to blobs automatically or they may be externally provided. It is
// the responsibility of the consumer to determine which signatures to
// consumer and which to ignore.
package distribution
