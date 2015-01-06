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

A **manifest** is a signed document identifying a particular named resource. It is comprised of sections which can be of different types. In a first version, we only expect to be dealing with Docker images, which manifests may only hold an "image" section (to be specified separately, but that we'll assume to define a list of layers).

## Examples

Creating and pushing a Docker image:

```
$> dist put content1.tar
# dbe80f010ab3c73df8f3f6a54e161eecdf561f4d
$> dist put content2.tar
d37a5bcd9fb3b61675727ff0ae9db240ac520595
$> dist pack -p -t image icecrime/my_image dbe80f010ab3c73df8f3f6a54e161eecdf561f4d d37a5bcd9fb3b61675727ff0ae9db240ac520595
{
	"name": "icecrime/my_image",
	"schemaVersion": 1,
	"signatures": [
		[...]
	],
	[...]
}
$> dist push icecrime/my_image
```

## Command reference

### `pack` subcommand

```
Usage: dist pack [OPTIONS] identifier [ARGUMENTS]

Generate a new manifest, or append to an existing one, and sign the result

  -a,--append		Append to an existing manifest (<identifier> must be a valid manifest)
  -k,--key=<keyid>	Key to sign the resulting manifest
  -p,--print=false	Print the resulting manifest to stdout
  -t,--type=image	Section type to create (image is the only supported value today)
```

The `pack` subcommand is used to create and alter manifests. The expected list of arguments depends on the type of section being created.

When the command succeeds and `-a` was not specified, a new manifest is created. It contains:

* Common top-level attributes shared by all manifests (although the exact manifest specification is out of the scope of this document we expect to find the following: `"name"`, `"schemaVersion"`, `"signature"`)
* A section which structure is dependent on the content type specified using `-t`. As an example, and once again keeping in mind that the exact manifest layout is outside the scope of this document, a Docker image content section may hold a combination of filesystem layers identifiers, a runtime configuration object, ... In the future we could imagine extending this model to describe other type of content, such as source packages.

When the command succeeds and `-a` was specified, a new section is appended to the previously existing manifest, and the result is signed again.

***Note:** considering the amount of arguments that may be necessary to populate a single section, command-line arguments are probably not the proper way to go. Another possibility (@stevvooe) is to accept key=objectid arguments where objectid refer to JSON documents previously inserted through `put` calls. My take on this: I'd rather limit the number of necessary steps to generate a simple manifest.*

### `pull` subcommand

```
Usage: dist pull [OPTIONS] identifier

Pull and verify signed content from a registry

  -r,--registry="hub.docker.io"		The registry to use (e.g.: localhost:5000)

```

The `pull` subcommand is responsible for the following:

  1. Retrieving the manifest for resource `identifier` from the registry
  2. Retrieving the content in a manner dependent on the nature of sections found in the manifest. For an image section, content retrieval will consist of downloading layers which are missing locally. The command will abort if the retrieved manifest declares sections which type the client doesn't know how to handle.
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

## Open questions

* Do we want an extendable local storage system (i.e.: disk, DB, whatever, ...)?
* How would the Docker engine interact with the `dist` binary, and especially how do make a runtime container of a downloaded image without ruining performance?
