---
description: High level discussion of garbage collection
keywords: registry, garbage, images, tags, repository, distribution
title: Garbage collection
---

{% include registry.md %}

As of v2.4.0 a garbage collector command is included within the registry binary.
This document describes what this command does and how and why it should be used.

## About garbage collection

In the context of the Docker registry, garbage collection is the process of
removing blobs from the filesystem when they are no longer referenced by a
manifest. Blobs can include both layers and manifests.

Registry data can occupy considerable amounts of disk space. In addition,
garbage collection can be a security consideration, when it is desirable to ensure
that certain layers no longer exist on the filesystem.

## Garbage collection in practice

Filesystem layers are stored by their content address in the Registry. This
has many advantages, one of which is that data is stored once and referred to by manifests.
See [here](compatibility.md#content-addressable-storage-cas) for more details.

Layers are therefore shared amongst manifests; each manifest maintains a reference
to the layer. As long as a layer is referenced by one manifest, it cannot be garbage
collected.

Manifests and layers can be `deleted` with the registry API (refer to the API
documentation [here](spec/api.md#deleting-a-layer) and
[here](spec/api.md#deleting-an-image) for details). This API removes references
to the target and makes them eligible for garbage collection. It also makes them
unable to be read via the API.

If a layer is deleted, it is removed from the filesystem when garbage collection
is run. If a manifest is deleted the layers to which it refers are removed from
the filesystem if no other manifests refers to them.


### Example

In this example manifest A references two layers: `a` and `b`. Manifest `B` references
layers `a` and `c`. In this state, nothing is eligible for garbage collection:

```
A -----> a <----- B
    \--> b     |
         c <--/
```

Manifest B is deleted via the API:

```
A -----> a     B
    \--> b
         c
```

In this state layer `c` no longer has a reference and is eligible for garbage
collection. Layer `a` had one reference removed but not garbage
collected as it is still referenced by manifest `A`. The blob representing
manifest `B` is eligible for garbage collection.

After garbage collection has been run, manifest `A` and its blobs remain.

```
A -----> a
    \--> b
```


### More details about garbage collection

Garbage collection runs in two phases. First, in the 'mark' phase, the process
scans all the manifests in the registry. From these manifests, it constructs a
set of content address digests. This set is the 'mark set' and denotes the set
of blobs to *not* delete. Secondly, in the 'sweep' phase, the process scans all
the blobs and if a blob's content address digest is not in the mark set, the
process deletes it.


> **Note**: You should ensure that the registry is in read-only mode or not running at
> all. If you were to upload an image while garbage collection is running, there is the
> risk that the image's layers are mistakenly deleted leading to a corrupted image.

This type of garbage collection is known as stop-the-world garbage collection.

## Run garbage collection

Garbage collection can be run as follows

`bin/registry garbage-collect [--dry-run] /path/to/config.yml`

The garbage-collect command accepts a `--dry-run` parameter, which prints the progress
of the mark and sweep phases without removing any data. Running with a log level of `info`
gives a clear indication of items eligible for deletion.

The config.yml file should be in the following format:

```yaml
version: 0.1
storage:
  filesystem:
    rootdirectory: /registry/data
```

_Sample output from a dry run garbage collection with registry log level set to `info`_

```
hello-world
hello-world: marking manifest sha256:fea8895f450959fa676bcc1df0611ea93823a735a01205fd8622846041d0c7cf
hello-world: marking blob sha256:03f4658f8b782e12230c1783426bd3bacce651ce582a4ffb6fbbfa2079428ecb
hello-world: marking blob sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4
hello-world: marking configuration sha256:690ed74de00f99a7d00a98a5ad855ac4febd66412be132438f9b8dbd300a937d
ubuntu

4 blobs marked, 5 blobs eligible for deletion
blob eligible for deletion: sha256:28e09fddaacbfc8a13f82871d9d66141a6ed9ca526cb9ed295ef545ab4559b81
blob eligible for deletion: sha256:7e15ce58ccb2181a8fced7709e9893206f0937cc9543bc0c8178ea1cf4d7e7b5
blob eligible for deletion: sha256:87192bdbe00f8f2a62527f36bb4c7c7f4eaf9307e4b87e8334fb6abec1765bcb
blob eligible for deletion: sha256:b549a9959a664038fc35c155a95742cf12297672ca0ae35735ec027d55bf4e97
blob eligible for deletion: sha256:f251d679a7c61455f06d793e43c06786d7766c88b8c24edf242b2c08e3c3f599
```
