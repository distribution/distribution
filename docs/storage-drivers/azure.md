---
description: Explains how to use the Azure storage drivers
keywords: registry, service, driver, images, storage,  azure
title: Microsoft Azure storage driver
---

{% include registry.md %}

An implementation of the `storagedriver.StorageDriver` interface which uses [Microsoft Azure Blob Storage](http://azure.microsoft.com/en-us/services/storage/) for object storage.

## Parameters

| Parameter     | Required | Description                                                                                                                                                                                                                                                         |
|:--------------|:---------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `accountname` | yes      | Name of the Azure Storage Account.                                                                                                                                                                                                                                  |
| `accountkey`  | yes      | Primary or Secondary Key for the Storage Account.                                                                                                                                                                                                                   |
| `container`   | yes      | Name of the Azure root storage container in which all registry data is stored. Must comply the storage container name [requirements](https://docs.microsoft.com/rest/api/storageservices/fileservices/naming-and-referencing-containers--blobs--and-metadata). For example, if your url is `https://myaccount.blob.core.windows.net/myblob` use the container value of `myblob`.|
| `realm`       | no       | Domain name suffix for the Storage Service API endpoint. For example realm for "Azure in China" would be `core.chinacloudapi.cn` and realm for "Azure Government" would be `core.usgovcloudapi.net`. By default, this is `core.windows.net`.                        |


## Related information

* To get information about
[azure-blob-storage](http://azure.microsoft.com/en-us/services/storage/), visit
the Microsoft website.
* You can use Microsoft's [Blob Service REST API](https://msdn.microsoft.com/en-us/library/azure/dd135733.aspx) to [create a storage container](https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx).
