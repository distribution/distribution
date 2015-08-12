<!--[metadata]>
+++
title = "Insecure Registry"
description = "Deploying an insecure Registry"
keywords = ["registry, images, repository"]
+++
<![end-metadata]-->

# Insecure Registry

While it's highly recommended to secure your registry using a TLS certificate issued by a known CA, you may alternatively decide to use self-signed certificates, or even use your registry over plain http.

You have to understand the downsides in doing so, and the extra burden in configuration.

## Deploying a plain HTTP registry

> :warning: it's not possible to use an insecure registry with basic authentication

This basically tells Docker to entirely disregard security for your registry.

1. edit the file `/etc/default/docker` so that there is a line that reads: `DOCKER_OPTS="--insecure-registry myregistrydomain.com:5000"` (or add that to existing `DOCKER_OPTS`)
2. restart your Docker daemon: on ubuntu, this is usually `service docker stop && service docker start`

**Pros:**

 - easy to configure
 
**Cons:**
 
 - very insecure
 - you have to configure every docker daemon that wants to access your registry 
  
## Using self-signed certificates

> :warning: using this along with basic authentication requires to **also** trust the certificate into the OS cert store for some versions of docker

Generate your own certificate:

    mkdir -p certs && openssl req \
      -newkey rsa:4096 -nodes -sha256 -keyout certs/domain.key \
      -x509 -days 365 -out certs/domain.crt

Be sure to use the name `myregistrydomain.com` as a CN.

Stop and restart your registry.

Then you have to instruct every docker daemon to trust that certificate. This is done by copying the `domain.crt` file to `/etc/docker/certs.d/myregistrydomain.com:5000/ca.crt` (don't forget to restart docker after doing so).

Stop and restart all your docker daemons.

**Pros:**

 - more secure than the insecure registry solution

**Cons:**

 - you have to configure every docker daemon that wants to access your registry

## Failing...

Failing to configure docker and trying to pull from a registry that is not using TLS will result in the following message:

```
FATA[0000] Error response from daemon: v1 ping attempt failed with error:
Get https://myregistrydomain.com:5000/v1/_ping: tls: oversized record received with length 20527. 
If this private registry supports only HTTP or HTTPS with an unknown CA certificate,please add 
`--insecure-registry myregistrydomain.com:5000` to the daemon's arguments.
In the case of HTTPS, if you have access to the registry's CA certificate, no need for the flag;
simply place the CA certificate at /etc/docker/certs.d/myregistrydomain.com:5000/ca.crt
```
