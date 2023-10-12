---
description: Explains how to use storage drivers
keywords: registry, on-prem, images, tags, repository, distribution, storage drivers, advanced
title: Registry storage driver
---

This document describes the registry storage driver model, implementation, and explains how to contribute new storage drivers.

## Provided drivers

This storage driver package comes bundled with several drivers:

- [inmemory](inmemory): A temporary storage driver using a local inmemory map. This exists solely for reference and testing.
- [filesystem](filesystem): A local storage driver configured to use a directory tree in the local filesystem.
- [s3](s3): A driver storing objects in an Amazon Simple Storage Service (S3) bucket.
- [azure](azure): A driver storing objects in [Microsoft Azure Blob Storage](https://azure.microsoft.com/en-us/services/storage/).
- [gcs](gcs): A driver storing objects in a [Google Cloud Storage](https://cloud.google.com/storage/) bucket.
- oss: *NO LONGER SUPPORTED*
- swift: *NO LONGER SUPPORTED*

## Storage driver API

The storage driver API is designed to model a filesystem-like key/value storage in a manner abstract enough to support a range of drivers from the local filesystem to Amazon S3 or other distributed object storage systems.

Storage drivers are required to implement the `storagedriver.StorageDriver` interface provided in `storagedriver.go`, which includes methods for reading, writing, and deleting content, as well as listing child objects of a specified prefix key.

Storage drivers are intended to be written in Go, providing compile-time
validation of the `storagedriver.StorageDriver` interface.

## Driver selection and configuration

The preferred method of selecting a storage driver is using the `StorageDriverFactory` interface in the `storagedriver/factory` package. These factories provide a common interface for constructing storage drivers with a parameters map. The factory model is based on the [Register](https://golang.org/pkg/database/sql/#Register) and [Open](https://golang.org/pkg/database/sql/#Open) methods in the builtin [database/sql](https://golang.org/pkg/database/sql) package.

Storage driver factories may be registered by name using the
`factory.Register` method, and then later invoked by calling `factory.Create`
with a driver name and parameters map. If no such storage driver can be found,
`factory.Create` returns an `InvalidStorageDriverError`.

## Driver contribution

New storage drivers are not currently being accepted.
See <https://github.com/distribution/distribution/issues/3988> for discussion.

There are forks of this repo that implement custom storage drivers.
These are not supported by the OCI distribution project.
The known forks are:

- Storj DCS: <https://github.com/storj/docker-registry>
- HuaweiCloud OBS: <https://github.com/setoru/distribution/tree/obs>
- us3: <https://github.com/lambertxiao/distribution/tree/main>
- Baidu BOS: <https://github.com/dolfly/distribution/tree/bos>
- HDFS: <https://github.com/haosdent/distribution/tree/master>

### Writing new storage drivers

To create a valid storage driver, one must implement the
`storagedriver.StorageDriver` interface and make sure to expose this driver
via the factory system.

#### Registering

Storage drivers should call `factory.Register` with their driver name in an `init` method, allowing callers of `factory.New` to construct instances of this driver without requiring modification of imports throughout the codebase.

## Testing

Storage driver test suites are provided in
`storagedriver/testsuites/testsuites.go` and may be used for any storage
driver written in Go. Tests can be registered using the `RegisterSuite`
function, which run the same set of tests for any registered drivers.
