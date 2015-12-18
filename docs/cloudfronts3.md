### Cloudfront as Middleware /w S3 backend

# Create a Cloudfront distribution
The Cloudfront distribution must be created such that the path is set to the directory level of the root "docker" key in S3.

Defaults can be kept in most areas except:

Behaviors:
  - Viewer Protocol Policy: HTTPS Only
  - Allowed HTTP Methods: GET, HEAD, OPTIONS, PUT, POST, PATCH, DELETE
  - Cached HTTP Methods: OPTIONS (checked)
  - Restrict Viewer Access (Use Signed URLs or Signed Cookies): Yes
    - Trusted Signers: Self (Can add other accounts as long as you have access to Cloudfront Key Pairs for those additional accounts)

# Registry configuration
Here the `middleware` option is used. It is still important to keep the `storage` option as Cloudfront will only handle `pull` actions; `push` actions are still directly written to S3.

The following example shows what you will need at minimum:

example:
```
.
.
.
storage:
  s3:
    region: us-east-1
    bucket: docker.myregistry.com
middleware:
  storage:
    - name: cloudfront
      options:
        baseurl: https://abcdefghijklmn.cloudfront.net/
        privatekey: /etc/docker/cloudfront/pk-ABCEDFGHIJKLMNOPQRST.pem
        keypairid: ABCEDFGHIJKLMNOPQRST
.
.
.
```

# Cloudfront Key-Pair
A Cloudfront Key-Pair for the AWS accounts allowed access is required. For information, please see [Private Content Creating Key Pairs](http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-trusted-signers.html#private-content-creating-cloudfront-key-pairs).
