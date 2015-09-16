# Image Manifest Version 2, Schema 2

This document outlines the format of of the V2 image manifest, schema 2 version.
The original (and provisional) image manifest for V2 (schema 1), was introduced
in the Docker daemon in the [v1.3.0
release](https://github.com/docker/docker/commit/9f482a66ab37ec396ac61ed0c00d59122ac07453)
and is specified in the [schema 1 manifest definition](./manifest-v2-1.md)

# Distribution Content Manifests

A Content Manifest is a simple JSON file which contains general fields that
describe an object and its dependencies. The goal is for these manifests to
describe a reference to any application data, optional metadata or labels, and
to reference any dependent objects in a content-addressable and verifiable way.
These manifests can also be stored in a repository and referenced by digest.

## *Manifest* Field Descriptions

- **`schemaVersion`** *int*
	
   This field specifies the image manifest schema version as an integer. This
   document describes version `2` only.

- **`target`** *object*

    The target field may describe *any* object stored in a repository, allowing
    this manifest format (along with the generic dependencies list below) to
    support any type of application that can be represented as a configuration
    file and a collection of content-addressable blobs of data.

    Fields of a descriptor object are:
    
    - **`mediaType`** *string*
    
        The MIME type of the referenced object.
    
    - **`size`** *int*
    
        The size in bytes of the object. This field exists so that a client
        will have an expected size for the content before validating. If the
        length of the retrieved content does not match the specified length,
        the content should not be trusted.
    
    - **`digest`** *string*

        The digest of the content, as defined by the
        [Registry V2 HTTP API Specificiation](https://docs.docker.com/registry/spec/api/#digest-parameter).

	- **`labels`** *object*

        Labels may be keyed to *any* JSON object, allowing content creators to
        annotate their manifest with any additional metadata beyond those already
        defined by this manifest format or contained within the target object. 

- **`dependencies`** *array*
	
    The dependencies array contains descriptor objects of the same form as the
    `target` field, normally used to represent the collection of content-addressable
    blobs of data associated with the target manifest, but can represent any
    object stored in a repository for flexibility.

## Example Manifest

*Example showing a simple Docker v1 image manifest and three dependent content blobs (layers)*

```json
{
    "schemaVersion": 2,
    "target": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "size": 7023,
        "digest": "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7",
        "labels": {
            "createdAt": "2015-09-16T16:29:07.952493971-07:00",
            "version": "3.1.4-a159+265"
         }
    },
    "dependencies": [
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "size": 32654,
            "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
        },
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "size": 16724,
            "digest": "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b"
        },
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "size": 73109,
            "digest": "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736"
        }
    ],
}
```
*Example showing a proposed "fat manifest" Docker v2 image using dependency entries to link
to manifests representing different architecture variants available for this repository manifest.*

*Note: A parsing of this manifest would require dereferencing the appropriate dependency manifest
for a specific architecture, pulling that manifest, and then parsing its dependencies which would
reference the actual layer content (for our multi-arch example here) as content-addressable blobs
which would form the image for a specific architecture.*

```json
{
    "schemaVersion": 2,
    "target": {
        "mediaType": "application/vnd.docker.container.image.v2",
        "size": 7023,
        "digest": "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7"
    },
    "dependencies": [
        {
            "mediaType": "application/vnd.docker.container.image.v1+json",
            "size": 7143,
            "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
            "labels": {
                "os": "linux",
                "arch": "ppc64le"
             }
        },
        {
            "mediaType": "application/vnd.docker.container.image.v1+json",
            "size": 7144,
            "digest": "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
            "labels": {
                "os": "linux",
                "arch": "amd64"
             }
        },
        {
            "mediaType": "application/vnd.docker.container.image.v1+json",
            "size": 7141,
            "digest": "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
            "labels": {
                "os": "linux",
                "arch": "s390x"
             }
        }
    ],
}
```
