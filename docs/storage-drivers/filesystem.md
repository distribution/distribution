<!--[metadata]>
+++
title = "Filesystem storage driver"
description = "Explains how to use the filesystem storage drivers"
keywords = ["registry, service, driver, images, storage,  filesystem"]
+++
<![end-metadata]-->


# Filesystem storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses the local filesystem.

## Parameters

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to `/var/lib/registry`.
