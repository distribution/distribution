<!--[metadata]>
+++
title = "Kingsoft Cloud KS3 storage driver"
description = "Explains how to use the Kingsoft Cloud KS3 storage drivers"
keywords = ["registry, service, driver, images, storage, KS3, Kingsoft Cloud"]
[menu.main]
parent="smn_storagedrivers"
+++
<![end-metadata]-->


# Kingsoft KS3 storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses [Kingsoft Cloud KS3](http://ks3.ksyun.com/index.html) for object storage.

## Parameters

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>accesskey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Your KS3 Access Key.
    </td>
  </tr>
  <tr>
    <td>
      <code>secretkey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Your KS3 Secret Key.
    </td>
  </tr>
  <tr>
    <td>
      <code>region</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The name of the KS3 region in which you would like to store objects (for example `ks3-cn-beijing`).
      For a list of regions, you can look at http://ks3.ksyun.com/doc/api/index.html.
    </td>
  </tr>
  <tr>
    <td>
      <code>regionendpoint</code>
    </td>
    <td>
      no
    </td>
    <td>
      An endpoint which defaults to `<bucket>.<region>.ksyun.com` or `<bucket>.<region>-internal-ksyun.com` (when `internal=true`).
      You can change the default regionendpoint by changing this value.
    </td>
  </tr>
  <tr>
    <td>
      <code>internal</code>
    </td>
    <td>
      no
    </td>
    <td>
      An internal endpoint or the public endpoint for KS3 access. The default is false.
    </td>
  </tr>
  <tr>
    <td>
      <code>bucket</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The name of your KS3 bucket in which you want to store the registry's data.
    </td>
  </tr>
  <tr>
    <td>
      <code>encrypt</code>
    </td>
    <td>
      no
    </td>
    <td>
       Specifies whether the registry stores the image in encrypted format or
       not. A boolean value. The default is false.
    </td>
  </tr>
  <tr>
    <td>
      <code>secure</code>
    </td>
    <td>
      no
    </td>
    <td>
      Indicates whether to use HTTPS instead of HTTP. A boolean value. The
      default is <code>true</code>.
    </td>
  </tr>
  <tr>
    <td>
      <code>chunksize</code>
    </td>
    <td>
      no
    </td>
    <td>
      The KS3 API requires multipart upload chunks to be at least 5MB. This value
      should be a number that is larger than 5*1024*1024.
    </td>
  </tr>
  <tr>
    <td>
      <code>rootdirectory</code>
    </td>
    <td>
      no
    </td>
    <td>
      This is a prefix that will be applied to all KS3 keys to allow you to segment data in your bucket if necessary.
    </td>
  </tr>
</table>


`accesskey`: Your ks3 access key.

`secretkey`: Your ks3 secret key.

**Note** You can provide empty strings for your access and secret keys if you set the following environment variables:

```
KS3_ACCESS_KEY_ID=MY-ACCESS-KEY
KS3_SECRET_ACCESS_KEY=MY-SECRET-KEY
```

`region`: The name of the ks3 region in which you would like to store objects. You can find the region name in the following table which provided by Kingsoft Cloud KS3 storage service. Note that you can't describe or access additional regions.

| Code | Name |
| ---- | ---- |
| ks3-cn-hangzhou | Hangzhou |
| ks3-cn-beijing | Beijing |
| ks3-cn-shanghai | Shanghai |
| ks3-cn-hk-1 | Hongkong |
| ks3-us-west-1 | UsWest1|

`regionendpoint`: (optional) You can change the default regionendpoint by changing this value. This should not be provided when using Kingsoft Cloud KS3 default region endpoint.

`bucket`: The name of your KS3 bucket where you wish to store objects. The bucket must exist prior to the driver initialization.

`encrypt`: (optional) Whether you would like your data encrypted on the server side (defaults to false if not specified).

`secure`: (optional) Whether you would like to transfer data to the bucket over ssl or not. Defaults to true (meaning transferring over ssl) if not specified. Note that while setting this to false will improve performance, it is not recommended due to security concerns.

`chunksize`: (optional) The default part size for multipart uploads (performed by WriteStream) to KS3. The default is 10 MB. Keep in mind that the minimum part size for KS3 is 5MB. Depending on the speed of your connection to KS3, a larger chunk size may result in better performance; faster connections will benefit from larger chunk sizes.

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (bucket root).
