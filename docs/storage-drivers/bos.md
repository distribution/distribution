<!--[metadata]>
+++
title = "Baidu BOS storage driver"
description = "Explains how to use the Baidu BOS storage driver"
keywords = ["registry, service, driver, images, storage, BOS, baidu"]
[menu.main]
parent="smn_storagedrivers"
+++
<![end-metadata]-->

# Baidu BOS storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses [Baidu BOS](https://cloud.baidu.com/product/bos.html) for object storage.

## Parameters

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
<tr>
  <td>
    <code>accesskeyid</code>
</td>
<td>
yes
</td>
<td>
Your access key ID.
</td>
</tr>
<tr>
  <td>
    <code>accesskeysecret</code>
</td>
<td>
yes
</td>
<td>
Your access key secret.
</td>
</tr>
<tr>
  <td>
    <code>region</code>
</td>
<td>
yes
</td>
<td> The name of the BOS region in which you would like to store objects (for example `bj`). For a list of regions, you can look at (https://cloud.baidu.com/doc/BOS/DevRef.html#BOS.E8.AE.BF.E9.97.AE.E5.9F.9F.E5.90.8D) 
</td>
</tr>
<tr>
  <td>
    <code>bucket</code>
</td>
<td>
yes
</td>
<td> The name of your BOS bucket where you wish to store objects (needs to already be created prior to driver initialization).
</td>
</tr>
<tr>
  <td>
    <code>endpoint</code>
</td>
<td>
no
</td>
<td>
An endpoint which defaults to `bj.bcebos.com`. You can change the default endpoint by changing this value. For a list of endpoints, you can look at (https://cloud.baidu.com/doc/BOS/DevRef.html#BOS.E8.AE.BF.E9.97.AE.E5.9F.9F.E5.90.8D) 
</td>
</tr>
<tr>
  <td>
    <code>secure</code>
</td>
<td>
no
</td>
<td> Specifies whether to transfer data to the bucket over ssl or not. If you omit this value, `true` is used.
</td>
</tr>
<tr>
  <td>
    <code>chunksize</code>
</td>
<td>
no
</td>
<td> The default part size for multipart uploads (performed by WriteStream) to BOS. The default is 10 MB. Keep in mind that the minimum part size for BOS is 5MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to BOS.
</td>
</tr>
<tr>
  <td>
    <code>rootdirectory</code>
</td>
<td>
no
</td>
<td> The root directory tree in which to store all registry files. Defaults to an empty string (bucket root).
</td>
</tr>
</table>
