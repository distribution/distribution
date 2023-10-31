---
description: Explains how to use the rewrite storage middleware
keywords: registry, service, driver, images, storage, middleware, rewrite
title: Rewrite middleware
---

A storage middleware which allows to rewrite the URL returned by the storage driver.

For example, it can be used to rewrite the Blob Storage URL returned by the Azure Blob Storage driver to use Azure CDN.

## Parameters

* `scheme`: (optional): Rewrite the returned URL scheme (if set).
* `host`: (optional): Rewrite the returned URL host (if set).
* `trimpathprefix` (optional): Trim the prefix from the returned URL path (if set).

## Example configuration

```yaml
storage:
  azure:
    accountname: "ACCOUNT_NAME"
    accountkey: "******"
    container: container-name
middleware:
  storage:
    - name: rewrite
      options:
        scheme: https
        host: example-cdn-endpoint.azurefd.net
        trimpathprefix: /container-name
```
