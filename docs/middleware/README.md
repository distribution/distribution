Metadata Store
==============

The metadata store middleware saves tag and manifest information to RethinkDB.
This gives us many benefits over distribution's standard method of saving
metadata on the filesystem:

- Our APIs can be more verbose, showing architecture, OS, author, push time etc.
  for each tag and manifest
- Our APIs for listing tags are much faster, as it doens't depend on reads over
  a remote distributed filesystem
- GC's mark phase is much quicker; we list layers from the manifest table
- We can delete V2 manifests by tags (CAS dictates that if two tags refer to the
  same image they'll use the same manifest. Therefore manifests should only be
  deleted if there's one tag pointing to it)

**NOTE**: The filesystem is still used for all read operations. This guarantees
that pulls work during the migration from 2.x to 2.1 — during this time the
metadata store is empty therefore reading tags/manifests will fail.

## Spec

https://docs.google.com/document/d/1hv6bCqIlTb-lyeP5bL1Gy5xK-UgUJuPbD2y-GY21dMQ


### Tag deletion

Requirements for deleting tags:

- Deleting a tag must delete the tag's manifest *if no other tags refer to the
  manifest*.
- Deleting a tag must retain the manifest if other tags refer to the manifest

Tag deletion is implemented using a tombstone column within rethinkdb (soft
deletion).

Delete flow:

  1. Update the tag's deleted column in rethinkDB to `true`
    i. if this fails return an error; deletion did not work
  2. Attempt to delete the blob from the blobstore
    i. if this fails, attempt to delete from the blobstore during GC

This means that *the blobstore may be inconsistent with our database*. To
resolve this, all registry operations for reading tags during pulls should
attempt to read from RethinkDB first; if an error is returned *then* we should
attempt to read from the blobstore.

Affected:

- Fetching single tags: needs to check deleted column
- Fetching all repo's tags: needs to filter deleted column; only show undeleted
- Deleting tags: if the tag is the last reference to a manifest (last undeleted
  tag) we should mark the manifest as deleted
- Creating a tag: we need to upsert on tags. If the tag exists, set `deleted` to
  false in an update. Otherwise create a new row.

