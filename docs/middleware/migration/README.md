Migration
=========

Migrate all tag and manifest metadata into the new tag/metadata store using
rethinkdb defined within `manager/`.

## How?

Similar to mark and sweep:

1. Iterate through all repositories
2. For each repository, iterate through each tag
3. For each tag load the manifest and:
  1. store the manifest plus config blob metadata
  2. store the tag data

Once the migration completes update the `isRepoMetadataMigrated` flag (to be
renamed) to true.

## Notes

The tagstore middleware will ensure that any new pushes since migration starts
are properly inserted in the database. This means that we do not need to worry
about stale data from uploads started after the migration.

## Problems

**Resumes**

This needs to be interruptable; if the task fails we should start from where we
left off (or near); we shouldn't start from scratch.

In order to do this we store the name of the repository we're currently
migrating; we can iterate through all repositories until we reach the current
repository and then restart migration of all tags.

This is an easy and low-cost solution to resumes vs always saving the name of
the tags we're migrating.
