The `dist` client binary
========================

*Note: the binary name, like everything else, is subject to change.*

The `dist` tool is the entrypoint for packaging and shipping content. It accepts the following subcommands that will each be detailed in the following sections.

  - The lower-level `put`/`get` couple act **locally** on **opaque objects** identified by a **unique ID**
  - The higher-level `push`/`pull` couple interact with a **remote registry** on **content** described through a **manifest**

To be detailed:

  - The `pack` command is used to generate and modify resource manifests
  - The `list` subcommand lists locally available content
  - The `remove` deletes locally available content
  - The `login` subcommand authenticates to a remote registry

## Definitions

A **manifest** is a signed document identifying a particular named resource and its content. Content is described by:

  - An application-provided type (uninterpreted by the `dist` tool)
  - An application-provided JSON object (uninterpreted by the `dist` tool)
  - A list of dependent object ids for the `dist` tool to send at `push` time and fetch at `pull` time

The initial version of `dist` only manages Docker images where:

  - The type could for example be "docker-image 1.0"
  - The JSON descriptor object will comply with the Docker image format specification
  - Dependent objects are the tarballs representing the filesystem layers

## Examples

Creating and pushing a Docker image (the presented manifest is only provided as an example and will be the subject of a dedicated specification):

```
$> dist put content1.tar
# dbe80f010ab3c73df8f3f6a54e161eecdf561f4d
$> dist put content2.tar
d37a5bcd9fb3b61675727ff0ae9db240ac520595
$> dist pack -p image=my_image.json icecrime/my_image dbe80f010ab3c73df8f3f6a54e161eecdf561f4d d37a5bcd9fb3b61675727ff0ae9db240ac520595
{
	"name": "icecrime/my_image",
	"schemaVersion": 1,
	"signatures": [
		[...]
	],
    "content": [
        {
            "type": "image",
            "desc": {
                // <content of the 'my_image.json' file>
            },
            "data": [
                {
                    "type": "file",
                    "id": "dbe80f010ab3c73df8f3f6a54e161eecdf561f4d"
                },
                {
                    "type": "file",
                    "id": "d37a5bcd9fb3b61675727ff0ae9db240ac520595"
                }
            ]
        }
    ]
}
$> dist push icecrime/my_image
```

## Command reference

### `pack` subcommand

```
Usage: dist pack [OPTIONS] <content-type>=<content-descriptor-file> [dependent_object...]

Generate a new manifest, or append to an existing one, and sign the result

  -a,--append		Append to an existing manifest (<identifier> must be a valid manifest)
  -k,--key=<keyid>	Key to sign the resulting manifest
  -p,--print=false	Print the resulting manifest to stdout
```

The `pack` subcommand is used to create and alter manifests. The expected list of arguments depends on the type of section being created.

When the command succeeds and `-a` was not specified, a new manifest is created. It contains:

* Common top-level attributes shared by all manifests (the exact manifest specification is out of the scope of this document)
* A declaration for a content of type `<content_type>`, described by JSON object read in file `<content-descriptor-file>` (or stdin if filename is left empty), with the `<dependent_object>` being the identifiers of dependent objects as previously returned by `dist put`.

When the command succeeds and `-a` was specified, new content is appended to the previously existing manifest, and the result is signed again.

The `pack` subcommand makes no validation on the `<content-type>` string, and only ensures the descriptor object is valid JSON. Dependent objects must be valid object identifiers.

### `pull` subcommand

```
Usage: dist pull [OPTIONS] identifier

Pull and verify signed content from a registry

  -r,--registry="hub.docker.io"		The registry to use (e.g.: localhost:5000)

```

The `pull` subcommand is responsible for the following:

  1. Retrieving the manifest for resource `identifier` from the registry
  2. Retrieving dependent objects of each content declared in the manifest. It takes advantage of object being content-addressable to avoid downloading objects already available locally.
  3. Using the manifest signature to validate retrieved content.

When a `pull` action completes succesfully, new objects are made available locally for use with the `get` subcommand.

### `push` subcommand

```
Usage: dist push [OPTIONS] identifier

Push signed content to a registry

  -r,--registry="hub.docker.i"o" 	The registry to use (e.g.: localhost:5000)
```

### `get` subcommand

```
Usage: dist get [OPTIONS] objectid

Get content or information on a locally stored object

  -e,--exists	Suppress all output; instead exit with zero status if <objectid> exists and is a valid object
  -t,--type		Instead of the content, show the object type identified by <objectid>
  -s,--size		Instead of the content, show the object size identified by <objectid>
```

### `put` subcommand

```
Usage: dist put [OPTIONS] [file]

Insert an object in local storage. Content will be read from stdin when a content
<file> is not specified. The created object id will be printed to stdout.
```
