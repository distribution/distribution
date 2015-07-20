<!--[metadata]>
+++
title = "GCS storage driver"
description = "Explains how to use the Google Cloud Storage drivers"
keywords = ["registry, service, driver, images, storage,  gcs, google, cloud"]
+++
<![end-metadata]-->


# Google Cloud Storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses Google Cloud for object storage.

## Parameters

`bucket`: The name of your Google Cloud Storage bucket where you wish to store objects (needs to already be created prior to driver initialization).

`keyfile`: (optional) A private key file in JSON format, used for [Service Account Authentication](https://cloud.google.com/storage/docs/authentication#service_accounts).

**Note** Instead of a key file you can use [Google Application Default Credentials](https://developers.google.com/identity/protocols/application-default-credentials).

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (bucket root).
