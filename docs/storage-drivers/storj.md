---
description: Explains how to use the Storj DCS storage drivers
keywords: registry, service, driver, images, storage, Storj
title: Storj DCS storage driver
---

An implementation of the `storagedriver.StorageDriver` interface which uses
Storj DCS services for object storage.

## Parameters

| Parameter     | Required | Description                                                                                                                                                                                                                                                         |
|:--------------|:---------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `accessgrant` | yes | Your Access Grant which will be used to access Storj network. |
| `bucket`  | yes | The bucket name in which you want to store the registry's data. |

`accessgrant`: An Access Grant is a security envelope that contains a satellite address, a restricted API Key, and a set of one or more restricted prefix-based encryption keys—everything an application needs to locate an object on the network, access that object, and decrypt it.

`bucket`: The name of your Storj bucket where you wish to store objects. The bucket must exist prior to the driver initialization.

For more details how Storj DCS network works see this [overview](https://docs.storj.io/dcs/concepts/overview/).