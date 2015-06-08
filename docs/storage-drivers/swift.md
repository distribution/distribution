# Openstack Swift storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses [Openstack Swift](http://docs.openstack.org/developer/swift/) for object storage.

## Parameters

`authurl`: URL for obtaining an auth token.

`username`: Your Openstack user name.

`password`: Your Openstack password.

`container`: The name of your Swift container where you wish to store objects. An additional container - named `<container>_segments` to store the data will be used. The driver will try to create both containers during its initialization.

`tenant`: (optional) Your Openstack tenant name. You can either use `tenant` or `tenantid`.

`tenantid`: (optional) Your Openstack tenant id. You can either use `tenant` or `tenantid`.

`domain`: (Optional) Your Openstack domain name for Identity v3 API. You can either use `domain` or `domainid`.

`domainid`: (Optional) Your Openstack domain id for Identity v3 API. You can either use `domain` or `domainid`.

`insecureskipverify`: (Optional) insecureskipverify can be set to true to skip TLS verification for your openstack provider. Default is false.

`region`: (optional) The name of the Openstack region in which you would like to store objects (for example `fr`).

`chunksize`: (optional) The segment size for Dynamic Large Objects uploads (performed by WriteStream) to swift. The default is 5 MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to Swift.

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (container root).
