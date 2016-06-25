<!--[metadata]>
+++
title = "Web HDFS storage driver"
description = "Explains how to use the Web HDFS storage drivers"
keywords = ["registry, service, driver, images, storage, HDFS"]
+++
<![end-metadata]-->


# Web HDFS storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses [Hadoop HDFS Web API](http://hadoop.apache.org/docs/current/hadoop-project-dist/hadoop-hdfs/WebHDFS.html) for object storage.

## Parameters

`namenodehost`: the HDFS NameNode host that manages the file system metadata and DataNodes that store the actual data.

`namenodeport`: the HDFS NameNode port. 

`rootdirectory`: The root directory tree on HDFS in which all registry files will be stored. 

`username`: (optional) The authenticated user name. The default value is the current user name of distribution.

`blocksize`: (optional) HDFS block size. The default value is 128MB.

`buffersize`: (optional) buffer size for WebHDFS request. The default value is 16MB.

`replication`: (optional) replication number for WebHDFS request. The default value is 1.

**Note** Please use Apache Hadoop upper than version 2.6.0. And please add Datanodes host name to registry container `/etc/hosts`.
