<!--GITHUB
page_title: Filesystem storage driver
page_description: Explains how to use the filesystem storage drivers
page_keywords: registry, service, driver, images, storage, filesystem
IGNORES-->

# Filesystem storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses the local filesystem.

## Parameters

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to `/tmp/registry/storage`.
