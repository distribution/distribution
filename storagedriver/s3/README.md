Docker-Registry S3 Storage Driver
=========================================

An implementation of the `storagedriver.StorageDriver` interface which uses Amazon S3 for object storage.

## Parameters

`accesskey`: Your aws access key.

`secretkey`: Your aws secret key.

**Note** You can provide empty strings for your access and secret keys if you plan on running the driver on an ec2 instance and will handle authentication with the instance's credentials.

`region`: The name of the aws region in which you would like to store objects (for example `us-east-1`). For a list of regions, you can look at http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html

`bucket`: The name of your s3 bucket where you wish to store objects (needs to already be created on driver initialization).

`encrypt`: (optional) Whether you would like your data encrypted while it is being transfered (defaults to true if not specified).

`secure`: (optional) Whether you would like to transfer data over ssl or not. Defaults to true (meaning transfering over ssl) if not specified. Note that while setting this to false will improve performance, it is not recommended due to security concerns.

`rootdirectory`: (optional) The root directory tree in which all registry files will be stored. Defaults to the empty string.