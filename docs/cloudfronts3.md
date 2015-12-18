# Cloudfront as Middleware /w S3 backend

## Use Case
Adding Cloudfront as a middleware for your registry can dramatically improve pull times. Your registry will have the ability to retrieve your images from edge servers, rather than the geographically limited location of your s3 bucket. The farther your registry is from your bucket, the more improvements you will see. See [Amazon Cloudfront](https://aws.amazon.com/cloudfront/details/).

## Configuring Cloudfront for Distribution
If you are unfamiliar with creating a Cloudfront distribution, see [Getting Started with Cloudfront](http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/GettingStarted.html).

Defaults can be kept in most areas except:

#### Origin:

The Cloudfront distribution must be created such that the `Origin Path` is set to the directory level of the root "docker" key in S3. If your registry exists on the root of the bucket, this path should be left blank.

#### Behaviors:
  - Viewer Protocol Policy: HTTPS Only
  - Allowed HTTP Methods: GET, HEAD, OPTIONS, PUT, POST, PATCH, DELETE
  - Cached HTTP Methods: OPTIONS (checked)
  - Restrict Viewer Access (Use Signed URLs or Signed Cookies): Yes
    - Trusted Signers: Self (Can add other accounts as long as you have access to Cloudfront Key Pairs for those additional accounts)

## Registry configuration
Here the `middleware` option is used. It is still important to keep the `storage` option as Cloudfront will only handle `pull` actions; `push` actions are still directly written to S3.

The following example shows what you will need at minimum:
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

## Cloudfront Key-Pair
A Cloudfront key-pair is required for all AWS accounts needing access to your cloudfront distribution. For information, please see [Private Content Creating Key Pairs](http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-trusted-signers.html#private-content-creating-cloudfront-key-pairs).
