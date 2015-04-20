<!--GITHUB
page_title: Docker Registry Storage Driver
page_description: Explains how to use the storage drivers
page_keywords: registry, service, driver, images, storage
IGNORES-->

# Docker Registry Storage Driver

This document describes the registry storage driver model, implementation, and explains how to contribute new storage drivers.

## Provided Drivers

This storage driver package comes bundled with several drivers:

- [inmemory](storage-drivers/inmemory.md): A temporary storage driver using a local inmemory map. This exists solely for reference and testing.
- [filesystem](storage-drivers/filesystem.md): A local storage driver configured to use a directory tree in the local filesystem.
- [s3](storage-drivers/s3.md): A driver storing objects in an Amazon Simple Storage Solution (S3) bucket.
- [azure](storage-drivers/azure.md): A driver storing objects in [Microsoft Azure Blob Storage](http://azure.microsoft.com/en-us/services/storage/).

## Storage Driver API

The storage driver API is designed to model a filesystem-like key/value storage in a manner abstract enough to support a range of drivers from the local filesystem to Amazon S3 or other distributed object storage systems.

Storage drivers are required to implement the `storagedriver.StorageDriver` interface provided in `storagedriver.go`, which includes methods for reading, writing, and deleting content, as well as listing child objects of a specified prefix key.

Storage drivers are intended (but not required) to be written in go, providing compile-time validation of the `storagedriver.StorageDriver` interface, although an IPC driver wrapper means that it is not required for drivers to be included in the compiled registry. The `storagedriver/ipc` package provides a client/server protocol for running storage drivers provided in external executables as a managed child server process.

## Driver Selection and Configuration

The preferred method of selecting a storage driver is using the `StorageDriverFactory` interface in the `storagedriver/factory` package. These factories provide a common interface for constructing storage drivers with a parameters map. The factory model is based off of the [Register](http://golang.org/pkg/database/sql/#Register) and [Open](http://golang.org/pkg/database/sql/#Open) methods in the builtin [database/sql](http://golang.org/pkg/database/sql) package.

Storage driver factories may be registered by name using the `factory.Register` method, and then later invoked by calling `factory.Create` with a driver name and parameters map. If no driver is registered with the given name, this factory will attempt to find an executable storage driver with the executable name "registry-storage-\<driver name\>" and return an IPC storage driver wrapper managing the driver subprocess. If no such storage driver can be found, `factory.Create` will return an `InvalidStorageDriverError`.

## Driver Contribution

### Writing new storage drivers
To create a valid storage driver, one must implement the `storagedriver.StorageDriver` interface and make sure to expose this driver via the factory system and as a distributable IPC server executable.

#### In-process drivers
Storage drivers should call `factory.Register` with their driver name in an `init` method, allowing callers of `factory.New` to construct instances of this driver without requiring modification of imports throughout the codebase.

#### Out-of-process drivers
As many users will run the registry as a pre-constructed docker container, storage drivers should also be distributable as IPC server executables. Drivers written in go should model the main method provided in `storagedriver/filesystem/registry-storage-filesystem/filesystem.go`. Parameters to IPC drivers will be provided as a JSON-serialized map in the first argument to the process. These parameters should be validated and then a blocking call to `ipc.StorageDriverServer` should be made with a new storage driver.

Out-of-process drivers must also implement the `ipc.IPCStorageDriver` interface, which exposes a `Version` check for the storage driver. This is used to validate storage driver api compatibility at driver load-time.

## Testing
Storage driver test suites are provided in `storagedriver/testsuites/testsuites.go` and may be used for any storage driver written in go. Two methods are provided for registering test suites, `RegisterInProcessSuite` and `RegisterIPCSuite`, which run the same set of tests for the driver imported or managed over IPC respectively.

## Drivers written in other languages
Although storage drivers are strongly recommended to be written in go for consistency, compile-time validation, and support, the IPC framework allows for a level of language-agnosticism. Non-go drivers must implement the storage driver protocol by mimicing StorageDriverServer in `storagedriver/ipc/server.go`. As the IPC framework is a layer on top of [docker/libchan](https://github.com/docker/libchan), this currently limits language support to Java via [ndeloof/chan](https://github.com/ndeloof/jchan) and Javascript via [GraftJS/jschan](https://github.com/GraftJS/jschan), although contributions to the libchan project are welcome.
