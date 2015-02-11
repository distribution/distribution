# Docker Registry Microsoft Azure Blob Storage Driver


An implementation of the `storagedriver.StorageDriver` interface which uses [Microsoft Azure Blob Storage][azure-blob-storage] for object storage.

## Parameters

The following parameters must be used to authenticate and configure the storage driver (case-sensitive):

* `accountname`: Name of the Azure Storage Account.
* `accountkey`: Primary or Secondary Key for the Storage Account.
* `container`: Name of the root storage container in which all registry data will be stored. Must comply the storage container name [requirements][create-container-api].


[azure-blob-storage]: http://azure.microsoft.com/en-us/services/storage/
[create-container-api]: https://msdn.microsoft.com/en-us/library/azure/dd179468.aspx