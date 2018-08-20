<!--GITHUB
page_title: Speedy storage driver
page_description: Explains how to use the Speedy storage driver
page_keywords: registry, service, driver, images, storage, speedy
IGNORES-->

# Speedy storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses
[Speedy][speedy] for storage backend.

## Parameters

The following parameters must be used to configure the storage driver
(case-sensitive):

* `storageurl`: The speedy(imageserver) address (e.g. http://127.0.0.1:6788 or http://127.0.0.1:6788;http://127.0.0.1:6789). 
* `chunksize`: Size of the written objects(units is MB, e.g. 4 is 4MB).
* `heartbeatinterval`: The interval of heartbeat (units is seconds, e.g. 2 is 2 seconds) 


[speedy]: https://github.com/jcloudpub/speedy
