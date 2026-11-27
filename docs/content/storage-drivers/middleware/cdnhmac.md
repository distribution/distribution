---
description: Explains how to use the cdnhmac storage middleware
keywords: registry, service, driver, images, storage, middleware, hmac, cloudflare
title: CDN HMAC middleware
---

A storage middleware which appends a timed-HMAC token to the URL returned by the storage
driver, so that a Cloudflare WAF rule calling `is_timed_hmac_valid_v0()` can validate the
redirect at the edge.

This is useful when a CDN hostname fronts the storage bucket and does not itself check the
storage provider's presigned URL signature.

## Origin requirements

The origin behind the CDN **must not validate the presigned signature**. A typical setup
is a Cloudflare R2 bucket served through an
[R2 custom domain](https://developers.cloudflare.com/r2/buckets/public-buckets/#custom-domains),
which serves objects without checking the query string.

The request flow is:

1. The storage driver presigns the URL (for S3, the `X-Amz-*` query parameters).
2. `rewrite` replaces the host with the CDN hostname and trims the bucket path prefix.
3. `cdnhmac` appends the token as the last query parameter.
4. A Cloudflare WAF rule validates the token and blocks the request if it is missing,
   invalid or expired.
5. The request reaches the origin (the R2 custom domain), which serves the object. The
   presigned parameters and the token pass through but are not checked.

The token is then the only access control on the CDN hostname. Keep the secret private, and
make sure the WAF rule covers every request to that hostname.

{{< hint type=warning >}}
An origin that validates AWS Signature Version 4 (for example AWS S3, or R2's S3 API
endpoint) rejects these requests with `SignatureDoesNotMatch`. SigV4 signs every query
parameter, as well as the host and path, so the extra token parameter invalidates the
signature. So do the host and path changes made by `rewrite`. Such an origin can only work
if the CDN restores the original host and path, and strips the token parameter, before
forwarding the request. This setup is not covered here.
{{< /hint >}}

## Token format

```
verify=<unix-seconds>-<base64url(HMAC-SHA256(secret, message + unix-seconds))>
```

The base64 uses the URL-safe alphabet with no padding, so the token needs no percent-encoding.
The WAF rule must therefore pass `'s'` as its `flags` argument.

`message` is everything in the request URI that precedes the token: the URL-escaped path,
plus the existing query string if there is one:

```
/blobs/sha256/ab/abcd/data?X-Amz-Algorithm=...&X-Amz-Signature=...
```

1. **The token is always the last parameter in the query string.** Cloudflare parses it
   positionally from the end of the URI.
2. **The existing query string is signed too**, and is preserved byte for byte.

List this middleware after `rewrite`, otherwise the message covers a URI the client never
requests and every validation fails.

## Parameters

* `secret` (required unless `secretfile` is set): the shared secret, inline. Mutually
  exclusive with `secretfile`.
* `secretfile` (required unless `secret` is set): path to a file holding the shared secret.
  Leading and trailing whitespace is stripped. Mutually exclusive with `secret`.
* `param` (optional): query parameter name for the token. Defaults to `verify`.

## Example configuration

```yaml
storage:
  s3:
    region: auto
    regionendpoint: https://s3.example.com
    bucket: registry
    forcepathstyle: true
middleware:
  storage:
    - name: rewrite
      options:
        scheme: https
        host: cdn.example.com
        trimpathprefix: /registry
    - name: cdnhmac
      options:
        secretfile: /etc/registry/cdn-hmac-secret
```

The matching Cloudflare WAF rule, with action `block`:

```
(http.host eq "cdn.example.com"
 and not is_timed_hmac_valid_v0("<secret>", http.request.uri, 3600, http.request.timestamp.sec, 8, "s"))
```

`8` is the length of the separator `&verify=`. Change it if `param` is changed.
