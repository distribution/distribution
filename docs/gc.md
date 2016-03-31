<!--[metadata]>
+++
title = "Garbage Collection"
description = "High level discussion of garabage collection"
keywords = ["registry, garbage, images, tags, repository, distribution"]
+++
<![end-metadata]-->

# What Garbage Collection Does

"Garbage collection deletes blobs which no manifests reference. Manifests and
blobs which are deleted by their digest through the Registry API will become
eligible for garbage collection, but the actual blobs will not be removed from
storage until garbage collection is run.

# How Garbage Collection Works

Garbage collection runs in two phases. First, in the 'mark' phase, the process
scans all the manifests in the registry. From these manifests, it constructs a
set of content address digests. This set is the 'mark set' and denotes the set
of blobs to *not* delete. Secondly, in the 'sweep' phase, the process scans all
the blobs and if a blob's content address digest is not in the mark set, the
process will delete it.

> **NOTE** You should ensure that the registry is in read-only mode or not running at
> all. If you were to upload an image while garbage collection is running, there is the
> risk that the image's layers will be mistakenly deleted, leading to a corrupted image.

This type of garbage collection is known as stop-the-world garbage collection.  In
future registry versions the intention is that garbage collection will be an 
automated background action and this manual process will no longer apply.

# How to Run

You can run garbage collection by running

`docker run --rm registry-image-name garbage-collect /etc/docker/registry/config.yml`

Additionally, garbage collection can be run in `dry-run` mode, which will print
the progress of the mark and sweep phases without removing any data.

