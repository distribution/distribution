<!--[metadata]>
+++
title = "Deploying a registry server"
description = "Explains how to deploy a registry server"
keywords = ["registry, service, images, repository"]
[menu.main]
parent="smn_registry"
weight=3
+++
<![end-metadata]-->

# Deploying a registry server

You need to [install Docker version 1.6.0 or newer](https://docs.docker.com/installation/).

## Running on localhost

Start your registry:

    docker run -d -p 5000:5000 --restart=always --name registry registry:2

You can now use it with docker.

Get any image from the hub and tag it to point to your registry:

    docker pull ubuntu && docker tag ubuntu localhost:5000/ubuntu

... then push it to your registry:

    docker push localhost:5000/ubuntu

... then pull it back from your registry:

    docker pull localhost:5000/ubuntu

To stop your registry, you would:

    docker stop registry && docker rm -v registry

## Storage

By default, your registry data is persisted as a [docker volume](https://docs.docker.com/userguide/dockervolumes/) on the host filesystem. Properly understanding volumes is essential if you want to stick with a local filesystem storage.

Specifically, you might want to point your volume location to a specific place in order to more easily access your registry data. To do so you can:

    docker run -d -p 5000:5000 --restart=always --name registry \
      -v `pwd`/data:/var/lib/registry \
      registry:2

### Alternatives

You should usually consider using [another storage backend](https://github.com/docker/distribution/blob/master/docs/storagedrivers.md) instead of the local filesystem. Use the [storage configuration options](https://github.com/docker/distribution/blob/master/docs/configuration.md#storage) to configure an alternate storage backend.

Using one of these will allow you to more easily scale your registry, and leverage your storage redundancy and availability features. 

## Running a domain registry

While running on `localhost` has its uses, most people want their registry to be more widely available. To do so, the Docker engine requires you to secure it using TLS, which is conceptually very similar to configuring your web server with SSL.

### Get a certificate

Assuming that you own the domain `myregistrydomain.com`, and that its DNS record points to the host where you are running your registry, you first need to get a certificate from a CA.

Move and/or rename your crt file to: `certs/domain.crt` - and your key file to: `certs/domain.key`.

Make sure you stopped your registry from the previous steps, then start your registry again with TLS enabled:

    docker run -d -p 5000:5000 --restart=always --name registry \
      -v `pwd`/certs:/certs \
      -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
      -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key \
      registry:2

You should now be able to access your registry from another docker host:

    docker pull ubuntu
    docker tag ubuntu myregistrydomain.com:5000/ubuntu
    docker push myregistrydomain.com:5000/ubuntu
    docker pull myregistrydomain.com:5000/ubuntu

#### Gotcha

A certificate issuer may supply you with an *intermediate* certificate. In this case, you must combine your certificate with the intermediate's to form a *certificate bundle*. You can do this using the `cat` command: 

    cat server.crt intermediate-certificates.pem > certs/domain.crt

### Alternatives

While rarely advisable, you may want to use self-signed certificates instead, or use your registry in an insecure fashion. You will find instructions [here](insecure.md).

## Restricting access

Except for registries running on secure local networks, registries should always implement access restrictions.

### Native basic auth

The simplest way to achieve access restriction is through basic authentication (this is very similar to other web servers' basic authentication mechanism).

:warning: You **cannot** use authentication with an insecure registry. You have to [configure TLS first](#running-a-domain-registry) for this to work.

First create a password file with one entry for the user "testuser", with password "testpassword":

    mkdir auth
    docker run --entrypoint htpasswd registry:2 -Bbn testuser testpassword > auth/htpasswd

Make sure you stopped your registry from the previous step, then start it again:

    docker run -d -p 5000:5000 --restart=always --name registry \
      -v `pwd`/auth:/auth \
      -e "REGISTRY_AUTH=htpasswd" \
      -e "REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm" \
      -e REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd \
      -v `pwd`/certs:/certs \
      -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
      -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key \
      registry:2

You should now be able to:

    docker login myregistrydomain.com:5000

And then push and pull images as an authenticated user.

### Alternatives

1. You may want to leverage more advanced basic auth implementations through a proxy design, in front of the registry. You will find an example of such design in the [nginx proxy documentation](nginx.md).

2. Alternatively, the Registry also supports delegated authentication, redirecting users to a specific, trusted token server. That approach requires significantly more investment, and only make sense if you want to fully configure ACLs and more control over the Registry integration into your global authorization and authentication systems.

You will find [background information here](spec/auth/token.md), and [configuration information here](configuration.md#auth).

Beware that you will have to implement your own authentication service for this to work.

## Managing with Compose

As your registry configuration grows more complex, dealing with it can quickly become tedious.

It's highly recommended to use [Docker Compose](https://docs.docker.com/compose/) to facilitate operating your registry. 

Here is a simple `docker-compose.yml` example that condenses everything explained so far:

```
registry:
  restart: always
  image: registry:2
  ports:
    - 5000:5000
  environment:
    REGISTRY_HTTP_TLS_CERTIFICATE: /certs/domain.crt
    REGISTRY_HTTP_TLS_KEY: /certs/domain.key
    REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY: /var/lib/registry
    REGISTRY_AUTH_HTPASSWD_PATH: /auth/htpasswd
    REGISTRY_AUTH_HTPASSWD_REALM: Registry Realm
  volumes:
    - /path/data:/var/lib/registry
    - /path/certs:/certs
    - /path/auth:/auth
```

:warning: replace `/path` by whatever directory that holds your `certs` and `auth` folder from above.

You can then start your registry with a simple

    docker-compose up -d

## Next

You will find more specific and advanced informations in the following sections:

 - [Configuration reference](configuration.md)
 - [Working with notifications](notifications.md)
 - [Registry API](spec/api.md)
 - [Storage driver model](storagedrivers.md)

<!--
 - [Glossary](glossary.md)
### Development resources
 - [Building the registry](building.md)
 - [Architecture notes](architecture.md)
 -->
