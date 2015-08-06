<!--[metadata]>
+++
title = "Aliyun OSS storage driver"
description = "Explains how to use the Aliyun OSS storage driver"
keywords = ["registry, service, driver, images, storage, OSS, aliyun"]
+++
<![end-metadata]-->

# Aliyun OSS storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses [Aliyun OSS](http://www.aliyun.com/product/oss) for object storage.

## Parameters

* `accesskeyid`: Your access key ID.

* `accesskeysecret`: Your access key secret.

* `region`: The name of the OSS region in which you would like to store objects (for example `oss-cn-beijing`). For a list of regions, you can look at <http://docs.aliyun.com/#/oss/product-documentation/domain-region>

* `endpoint`: (optional) By default, the endpoint shoulb be `<bucket>.<region>.aliyuncs.com` or `<bucket>.<region>-internal.aliyuncs.com` (when internal=true). You can change the default endpoint via changing this value.

* `internal`: (optional) Using internal endpoint or the public endpoint for OSS access. The default is false. For a list of regions, you can look at <http://docs.aliyun.com/#/oss/product-documentation/domain-region>

* `bucket`: The name of your OSS bucket where you wish to store objects (needs to already be created prior to driver initialization).

* `encrypt`: (optional) Whether you would like your data encrypted on the server side (defaults to false if not specified).

* `secure`: (optional) Whether you would like to transfer data to the bucket over ssl or not. Defaults to false if not specified.

* `chunksize`: (optional) The default part size for multipart uploads (performed by WriteStream) to OSS. The default is 10 MB. Keep in mind that the minimum part size for OSS is 5MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to OSS.

* `rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (bucket root).
