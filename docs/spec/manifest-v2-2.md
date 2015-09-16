# Distribution Content Manifests

A Content Manifest is a simple JSON file which contains general fields that
describe an object and its dependencies. The goal is for these manifests to
describe a reference to any application data, optional metadata or labels, and
to reference any dependent objects in a content-addressable and verifiable way.
These manifests can also be stored in a repository and referenced by digest.

## Field Descriptions

- **`schemaVersion`** *int*

    Specifies the schema version of the manifest as an integer. This document 
    describes version `2` only.

- **`target`** *object*

    The target field may describe *any* object stored in a repository, allowing
    this manifest format (along with the generic dependencies list below) to
    support any type of application that can be represented as a configuration
    file and a collection of content-addressable blobs of data.

    Fields are:
    
    - **`mediaType`** *string*
    
        The MIME type of the referenced object.
    
    - **`length`** *int*
    
        The length in bytes of the object. This field exists so that a client
        will have an expected size for the content before validating. If the
        length of the retrieved content does not match the specified length,
        the content should not be trusted.
    
    - **`digest`** *string*

        The digest of the content, as defined by the
        [Registry V2 HTTP API Specificiation](https://docs.docker.com/registry/spec/api/#digest-parameter).

- **`dependencies`** *array*

    Dependencies are an array of JSON objects which describe any content which
    the target depends on and can be found in the same repository. Those may be
    generic blobs of data or other content manifests with their own
    dependencies. The exact type and usage of the dependent objects is left to
    the target object data to describe.

    Fields are:

    - **`mediaType`** *string*
    
        Same as the `mediaType` field of the `target` object above.
    
    - **`length`** *int*
    
        Same as the `length` field of the `target` object above.
    
    - **`digest`** *string*
    
        Same as the `digest` field of the `target` object above.

    The ordering of these dependencies is not significant other than for
    consistent content-addressability. If the target object relies on any
    ordering for its dependencies it should enforce an ordering of its own
    separate from this list.

- **`labels`** *object*

    Labels may be keyed to *any* JSON object, allowing content creators to
    annotate their manifest with any additional metadata beyond those already
    defined by this manifest format or contained within the target object. 

### Example Manifest

The following manifest has a digest of
`sha256:289ba0d73cec55b385552af5fa82265a19911bbd641f871227ecaa96aadd358a`:

```json
{
    "schemaVersion": 2,
    "target": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "length": 7023,
        "digest": "sha256:b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7"
    },
    "dependencies": [
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "length": 32654,
            "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
        },
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "length": 16724,
            "digest": "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b"
        },
        {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-tar",
            "length": 73109,
            "digest": "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736"
        }
    ],
    "labels": {
        "createdAt": "2015-06-16T16:29:07.952493971-07:00",
        "version": "3.1.4-a159+265"
    }
}
```
