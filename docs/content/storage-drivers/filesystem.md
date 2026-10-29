---
description: Explains how to use the filesystem storage drivers
keywords: registry, service, driver, images, storage,  filesystem
title: Filesystem storage driver
---

An implementation of the `storagedriver.StorageDriver` interface which uses the local filesystem.

## Parameters

* `rootdirectory`: (optional) The absolute path to a root directory tree in which
to store all registry files. The registry stores all its data here so make sure
there is adequate space available. Defaults to `/var/lib/registry`. If the directory
does not exist, it will be created honoring [`umask`](https://man7.org/linux/man-pages/man2/umask.2.html)
bits. If `umask` bits are not set, the resulting permission will be `0777`.
* `maxthreads`: (optional) The maximum number of simultaneous blocking filesystem
operations permitted within the registry. Each operation spawns a new thread and
may cause thread exhaustion issues if many are done in parallel. Defaults to
`100`, and cannot be lower than `25`.
* `writebuffersize`: (optional) The size, in bytes, of the buffer the driver uses
when writing files. Defaults to `4096`, and cannot be lower than `4096`. See
[Write buffer size](#write-buffer-size).

## Write buffer size

The driver collects data written to a file in a buffer and passes it to the
filesystem whenever the buffer is full, and once more when the file is committed
or closed. A write that is larger than an empty buffer skips the buffer and is
passed to the filesystem directly. The registry writes blob uploads in chunks of
up to 32KiB, so with the default buffer of 4096 bytes uploads reach the
filesystem as writes of up to 32KiB. A `writebuffersize` larger than 32KiB
combines these chunks into fewer, larger writes, which can improve throughput on
filesystems that perform poorly with many small writes, such as some network
filesystems. A value between `4096` and 32KiB has little effect on blob uploads.

Each in-progress write, including every in-progress blob upload, allocates its
own buffer. The memory used for buffers is therefore about `writebuffersize`
multiplied by the number of concurrent uploads.

For example, to write blob uploads in chunks of 1MiB:

```yaml
storage:
  filesystem:
    rootdirectory: /var/lib/registry
    writebuffersize: 1048576
```

You can also set it with the `REGISTRY_STORAGE_FILESYSTEM_WRITEBUFFERSIZE`
environment variable.
