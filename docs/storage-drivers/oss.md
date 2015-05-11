<!--GITHUB
page_title: OSS storage driver
page_description: Explains how to use the OSS storage drivers
page_keywords: registry, service, driver, images, storage, OSS
IGNORES-->

# OSS storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses Aliyun OSS for object storage.

## Parameters

`accesskeyid`: Your access key ID.

`accesskeysecret`: Your access key secret.

`region`: The name of the aws region in which you would like to store objects (for example `oss-cn-beijing`). For a list of regions, you can look at http://docs.aliyun.com/?spm=5176.383663.9.2.0JzTP8#/oss/product-documentation/domain-region

`bucket`: The name of your OSS bucket where you wish to store objects (needs to already be created prior to driver initialization).

`chunksize`: (optional) The default part size for multipart uploads (performed by WriteStream) to OSS. The default is 10 MB. Keep in mind that the minimum part size for OSS is 5MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to OSS.

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (bucket root).
