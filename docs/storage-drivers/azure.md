---
description: Explains how to use the Azure storage drivers
keywords: registry, service, driver, images, storage,  azure
title: Microsoft Azure storage driver
---

An implementation of the `storagedriver.StorageDriver` interface which uses [Microsoft Azure Blob Storage](https://azure.microsoft.com/en-us/services/storage/) for object storage.

## Parameters

| Parameter                          | Required | Description                                                                                                                                                                                                                                                         |
|:-----------------------------------|:---------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `accountname`                      | yes      | Name of the Azure Storage Account.                                                                                                                                                                                                                                  |
| `accountkey`                       | yes      | Primary or Secondary Key for the Storage Account.                                                                                                                                                                                                                   |
| `container`                        | yes      | Name of the Azure root storage container in which all registry data is stored. Must comply the storage container name [requirements](https://docs.microsoft.com/rest/api/storageservices/fileservices/naming-and-referencing-containers--blobs--and-metadata). For example, if your url is `https://myaccount.blob.core.windows.net/myblob` use the container value of `myblob`.|
| `realm`                            | no       | Domain name suffix for the Storage Service API endpoint. For example realm for "Azure in China" would be `core.chinacloudapi.cn` and realm for "Azure Government" would be `core.usgovcloudapi.net`. By default, this is `core.windows.net`.                        |
| `copy_status_poll_max_retry`       | no       | Max retry number for polling of copy operation status. Retries use a simple backoff algorithm where each retry number is multiplied by `copy_status_poll_delay`, and this number is used as the delay. Set to -1 to disable retries and abort if the copy does not complete immediately. Defaults to 5.                |
| `copy_status_poll_delay`            | no       | Time to wait between retries for polling of copy operation status. This time is multiplied by N on each retry, where N is the retry number. Defaults to 100ms |


## Related information

* To get information about
[azure-blob-storage](https://azure.microsoft.com/en-us/services/storage/), visit
the Microsoft website.
* You can use Microsoft's [Blob Service REST API](https://docs.microsoft.com/en-us/rest/api/storageservices/Blob-Service-REST-API) to [create a storage container](https://docs.microsoft.com/en-us/rest/api/storageservices/Create-Container).
