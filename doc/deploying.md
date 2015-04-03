# Deploying

**TODO(stevvooe):** This should discuss various deployment scenarios for
production-ready deployments. These may be backed by ready-made docker images
but this should explain how they were created and what considerations were
present.


# Middleware Configuration

This section describes how to configure storage middleware in the registry to enable layers to be served via a CDN, thus reducing requests to the storage layer.  Currently [Amazon Cloudfront](http://aws.amazon.com/cloudfront/) is supported and must be used in conjunction with the S3 storage driver.

## Cloudfront

## Parameters

`name`: The name of the storage middleware.  Currently `cloudfront` is an accepted value.

`disabled`: This can be set to false to easily disable the middleware.

`options` : A set of key/value options to configure the middleware:

* `baseurl` : The cloudfront base URL
* `privatekey` : The location of your AWS private key on the filesystem 
* `keypairid` : The ID of your Cloudfront keypair.
* `duration` : The duration in minutes for which the URL is valid.  Default is 20.

Note: Cloudfront keys exist separately to other AWS keys.  See [here](http://docs.aws.amazon.com/AWSSecurityCredentials/1.0/AboutAWSCredentials.html#KeyPairs) for more information.

## Example



```
middleware:
    storage:
        - name: cloudfront
          disabled: false
          options:
             baseurl: http://d111111abcdef8.cloudfront.net
             privatekey: /path/to/asecret.pem
             keypairid: asecret
             duration: 60
```