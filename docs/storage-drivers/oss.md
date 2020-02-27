---
description: Explains how to use the Aliyun OSS storage driver
keywords: registry, service, driver, images, storage, OSS, aliyun
title: Aliyun OSS storage driver
---

An implementation of the `storagedriver.StorageDriver` interface which uses
[Aliyun OSS](https://www.alibabacloud.com/product/oss) for object storage.

## Parameters

| Parameter     | Required | Description |
|:--------------|:---------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `accesskeyid`  | yes | Your access key ID. |
| `accesskeysecret`  | yes | Your access key secret. |
| `region`  | yes | The name of the OSS region in which you would like to store objects (for example oss-cn-beijing). For a list of regions, you can look at the [official documentation](https://www.alibabacloud.com/help/doc-detail/31837.html). |
| `endpoint`  | no | An endpoint which defaults to `[bucket].[region].aliyuncs.com` or `[bucket].[region]-internal.aliyuncs.com` (when `internal=true`). You can change the default endpoint by changing this value. |
| `internal`  | no | An internal endpoint or the public endpoint for OSS access. The default is false. For a list of regions, you can look at the [official documentation](https://www.alibabacloud.com/help/doc-detail/31837.html). |
| `bucket`  |  yes | The name of your OSS bucket where you wish to store objects (needs to already be created prior to driver initialization). |
| `encrypt`  | no | Specifies whether you would like your data encrypted on the server side. Defaults to false if not specified. |
| `secure`  | no | Specifies whether to transfer data to the bucket over ssl or not. If you omit this value, `true` is used. |
| `chunksize`  | no | The default part size for multipart uploads (performed by WriteStream) to OSS. The default is 10 MB. Keep in mind that the minimum part size for OSS is 5MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to OSS. |
| `rootdirectory`  | no | The root directory tree in which to store all registry files. Defaults to an empty string (bucket root). |
