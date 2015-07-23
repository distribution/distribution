<!--[metadata]>
+++
title = "Swift storage driver"
description = "Explains how to use the OpenStack swift storage driver"
keywords = ["registry, service, driver, images, storage,  swift"]
+++
<![end-metadata]-->


# OpenStack Swift storage driver

An implementation of the `storagedriver.StorageDriver` interface that uses [OpenStack Swift](http://docs.openstack.org/developer/swift/) for object storage.

## Parameters

<table>
<tr>
  <td>
  <code>authurl</code>
  </td>
  <td>
    <p>URL for obtaining an auth token.</p>
  </td>
</tr>
<tr>
  <td>
  <code>username</code>
  </td>
  <td>
  <p>
  Your OpenStack user name.</p>
  </p>
  </td>
</tr>
<tr>
  <td>
  <code>password</code>
  <p>
  </td>
  <td>
  <p>
  Your OpenStack password.
  </p>
  </td>
</tr>
<tr>
  <td>
  <code>container</code>
  </td>
  <td>
  <p>
	  The name of your Swift container where you wish to store objects. An
  additional container called <code><container>_segments</code> stores the data
  is used. The driver creates both the named container and the segments
  container during its initialization.
  </p>
  </td>
</tr>
<tr>
  <td>
  <code>tenant</code>
  </td>
  <td>
  <p>
  Optionally, your OpenStack tenant name. You can either use <code>tenant</code> or <code>tenantid</code>.
  </p>
  </td>
</tr>
<tr>
    <td>
    <code>tenantid</code>
    </td>
    <td>
    <p>
    Optionally, your OpenStack tenant id. You can either use <code>tenant</code> or <code>tenantid</code>.
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>domain</code>
    </td>
    <td>
    <p>
    Optionally, your OpenStack domain name for Identity v3 API. You can either use <code>domain</code> or <code>domainid</code>.
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>domainid</code>
    </td>
    <td>
    <p>
    Optionally, your OpenStack domain id for Identity v3 API. You can either use <code>domain</code> or <code>domainid</code>.
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>insecureskipverify</code>
    </td>
    <td>
    <p>
    Optionally, set <code>insecureskipverify</code> to true to skip TLS verification for your OpenStack provider. The driver uses false by default.
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>region</code>
    </td>
    <td>
    <p>
    Optionally, specify the OpenStack region name in which you would like to store objects (for example <code>fr</code>).
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>chunksize</code>
    </td>
    <td>
    <p>
    Optionally, specify the segment size for Dynamic Large Objects uploads (performed by WriteStream) to Swift. The default is 5 MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to Swift.
    </p>
    </td>
</tr>
<tr>
    <td>
    <code>prefix</code>
    </td>
    <td>
    <p>
    Optionally, supply the root directory tree in which to store all registry files. Defaults to the empty string which is the container's root.</p>
    </p>
    </td>
</tr>
</table>