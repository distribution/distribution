# Openstack Swift storage driver

An implementation of the `storagedriver.StorageDriver` interface which uses Openstack Swift for object storage.

## Parameters

`authurl`: URL for obtaining an auth token.

`username`: Your Openstack user name.

`password`: Your Openstack password.

`container`: The name of your Swift container where you wish to store objects. An additional container - named `<container>_segments` to store the data will be used. The driver will try to create both containers during its initialization.

`tenant`: (optional) Your Openstack tenant name.

`region`: (optional) The name of the Openstack region in which you would like to store objects (for example `fr`).

`chunksize`: (optional) The segment size for Dynamic Large Objects uploads (performed by WriteStream) to swift. The default is 5 MB. You might experience better performance for larger chunk sizes depending on the speed of your connection to Swift.

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string (container root).
