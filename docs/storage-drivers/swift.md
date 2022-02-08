---
description: Explains how to use the OpenStack swift storage driver
keywords: registry, service, driver, images, storage, swift
title: OpenStack Swift storage driver
---

An implementation of the `storagedriver.StorageDriver` interface that uses
[OpenStack Swift](http://docs.openstack.org/developer/swift/) for object
storage.

## Parameters

| Parameter     | Required | Description                                                                                                                                                                                                                                                         |
|:--------------|:---------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `authurl`  |  yes  | URL for obtaining an auth token. https://storage.myprovider.com/v2.0 or https://storage.myprovider.com/v3/auth |
| `username`  |  yes  | Your Openstack user name. |
| `password`  |  yes | Your Openstack password. |
| `region`  | no   | The Openstack region in which your container exists. |
| `container`  |  yes  | The name of your Swift container where you wish to store the registry's data. The driver creates the named container during its initialization. |
| `tenant`  | no   | Your Openstack tenant name. You can either use `tenant` or `tenantid`. |
| `tenantid`  |  no | Your Openstack tenant name. You can either use `tenant` or `tenantid`. |
| `domain`  |  no  | Your Openstack domain name for Identity v3 API. You can either use `domain` or `domainid`. |
| `domainid`  | no   | Your Openstack domain name for Identity v3 API. You can either use `domain` or `domainid`. |
| `tenantdomain`  | no   | Your tenant's Openstack domain name for Identity v3 API. Only necessary if different from the <code>domain</code>. You can either use `tenantdomain` or `tenantdomainid`. |
| `tenantdomainid`  | no   | Your tenant's Openstack domain id for Identity v3 API. Only necessary if different from the <code>domain</code>. You can either use `tenantdomain` or `tenantdomainid`. |
| `trustid`  |  no  | Your Openstack trust ID for Identity v3 API. |
| `insecureskipverify`  | no   | Skips TLS verification if the value is wet to	`true`. The default is `false`. |
| `chunksize`  |  no  | Size of the data segments for the Swift Dynamic Large Objects. This value should be a number (defaults to 5M). |
| `prefix`  |  no  | This is a prefix that is applied to all Swift keys to allow you to segment data in your container if necessary. Defaults to the empty string which is the container's root. |
| `secretkey`  |  no  | The secret key used to generate temporary URLs. |
| `accesskey`  |  no  | The access key to generate temporary URLs. It is used by HP Cloud Object Storage in addition to the `secretkey` parameter. |
| `authversion`  | no  | Specify the OpenStack Auth's version, for example `3`. By default the driver autodetects the auth's version from the AuthURL. |
| `endpointtype`  | no   | The endpoint type used when connecting to swift. Possible values are `public`, `internal`, and `admin`. The default is `public`. |

The features supported by the Swift server are queried by requesting the `/info`
URL on the server. In case the administrator disabled that feature, the
configuration file can specify the following optional parameters :

|  Optional parameter | Description |
|:--------------|:---------|
| `tempurlcontainerkey`  |  Specify whether to use container secret key to generate temporary URL when set to true, or the account secret key otherwise. |
| `tempurlmethods`  |  Array of HTTP methods that are supported by the TempURL middleware of the Swift server. For example: `["GET", "PUT", "HEAD", "POST", "DELETE"]` |
