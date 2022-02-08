---
description: Explains how to deploy a registry
keywords: registry, on-prem, images, tags, repository, distribution, deployment
title: Deploy a registry server
---

Before you can deploy a registry, you need to install Docker on the host.
A registry is an instance of the `registry` image, and runs within Docker.

This topic provides basic information about deploying and configuring a
registry. For an exhaustive list of configuration options, see the
[configuration reference](configuration.md).

If you have an air-gapped datacenter, see
[Considerations for air-gapped registries](#considerations-for-air-gapped-registries).

## Run a local registry

Use a command like the following to start the registry container:

```console
$ docker run -d -p 5000:5000 --restart=always --name registry registry:2
```

The registry is now ready to use.

> **Warning**: These first few examples show registry configurations that are
> only appropriate for testing. A production-ready registry must be protected by
> TLS and should ideally use an access-control mechanism. Keep reading and then
> continue to the [configuration guide](configuration.md) to deploy a
> production-ready registry.

## Copy an image from Docker Hub to your registry

You can pull an image from Docker Hub and push it to your registry. The
following example pulls the `ubuntu:16.04` image from Docker Hub and re-tags it
as `my-ubuntu`, then pushes it to the local registry. Finally, the
`ubuntu:16.04` and `my-ubuntu` images are deleted locally and the
`my-ubuntu` image is pulled from the local registry.

1.  Pull the `ubuntu:16.04` image from Docker Hub.

    ```console
    $ docker pull ubuntu:16.04
    ```

2.  Tag the image as `localhost:5000/my-ubuntu`. This creates an additional tag
    for the existing image. When the first part of the tag is a hostname and
    port, Docker interprets this as the location of a registry, when pushing.

    ```console
    $ docker tag ubuntu:16.04 localhost:5000/my-ubuntu
    ```

3.  Push the image to the local registry running at `localhost:5000`:

    ```console
    $ docker push localhost:5000/my-ubuntu
    ```

4.  Remove the locally-cached `ubuntu:16.04` and `localhost:5000/my-ubuntu`
    images, so that you can test pulling the image from your registry. This
    does not remove the `localhost:5000/my-ubuntu` image from your registry.

    ```console
    $ docker image remove ubuntu:16.04
    $ docker image remove localhost:5000/my-ubuntu
    ```

5.  Pull the `localhost:5000/my-ubuntu` image from your local registry.

    ```console
    $ docker pull localhost:5000/my-ubuntu
    ```

## Stop a local registry

To stop the registry, use the same `docker container stop` command as with any other
container.

```console
$ docker container stop registry
```

To remove the container, use `docker container rm`.

```console
$ docker container stop registry && docker container rm -v registry
```

## Basic configuration

To configure the container, you can pass additional or modified options to the
`docker run` command.

The following sections provide basic guidelines for configuring your registry.
For more details, see the [registry configuration reference](configuration.md).

### Start the registry automatically

If you want to use the registry as part of your permanent infrastructure, you
should set it to restart automatically when Docker restarts or if it exits.
This example uses the `--restart always` flag to set a restart policy for the
registry.

```console
$ docker run -d \
  -p 5000:5000 \
  --restart=always \
  --name registry \
  registry:2
```

### Customize the published port

If you are already using port 5000, or you want to run multiple local
registries to separate areas of concern, you can customize the registry's
port settings. This example runs the registry on port 5001 and also names it
`registry-test`. Remember, the first part of the `-p` value is the host port
and the second part is the port within the container. Within the container, the
registry listens on port `5000` by default.

```console
$ docker run -d \
  -p 5001:5000 \
  --name registry-test \
  registry:2
```

If you want to change the port the registry listens on within the container, you
can use the environment variable `REGISTRY_HTTP_ADDR` to change it. This command
causes the registry to listen on port 5001 within the container:

```console
$ docker run -d \
  -e REGISTRY_HTTP_ADDR=0.0.0.0:5001 \
  -p 5001:5001 \
  --name registry-test \
  registry:2
```


## Storage customization

### Customize the storage location

By default, your registry data is persisted as a [docker volume](../storage/volumes.md)
on the host filesystem. If you want to store your registry contents at a specific
location on your host filesystem, such as if you have an SSD or SAN mounted into
a particular directory, you might decide to use a bind mount instead. A bind mount
is more dependent on the filesystem layout of the Docker host, but more performant
in many situations. The following example bind-mounts the host directory
`/mnt/registry` into the registry container at `/var/lib/registry/`.

```console
$ docker run -d \
  -p 5000:5000 \
  --restart=always \
  --name registry \
  -v /mnt/registry:/var/lib/registry \
  registry:2
```

### Customize the storage back-end

By default, the registry stores its data on the local filesystem, whether you
use a bind mount or a volume. You can store the registry data in an Amazon S3
bucket, Google Cloud Platform, or on another storage back-end by using
[storage drivers](./storage-drivers/index.md). For more information, see
[storage configuration options](./configuration.md#storage).

## Run an externally-accessible registry

Running a registry only accessible on `localhost` has limited usefulness. In
order to make your registry accessible to external hosts, you must first secure
it using TLS.

This example is extended in [Run the registry as a
service](#run-the-registry-as-a-service) below.

### Get a certificate

These examples assume the following:

- Your registry URL is `https://myregistry.domain.com/`.
- Your DNS, routing, and firewall settings allow access to the registry's host
  on port 443.
- You have already obtained a certificate from a certificate authority (CA).

If you have been issued an _intermediate_ certificate instead, see
[use an intermediate certificate](#use-an-intermediate-certificate).

1.  Create a `certs` directory.

    ```console
    $ mkdir -p certs
    ```

    Copy the `.crt` and `.key` files from the CA into the `certs` directory.
    The following steps assume that the files are named `domain.crt` and
    `domain.key`.

2.  Stop the registry if it is currently running.

    ```console
    $ docker container stop registry
    ```

3.  Restart the registry, directing it to use the TLS certificate. This command
    bind-mounts the `certs/` directory into the container at `/certs/`, and sets
    environment variables that tell the container where to find the `domain.crt`
    and `domain.key` file. The registry runs on port 443, the default HTTPS port.

    ```console
    $ docker run -d \
      --restart=always \
      --name registry \
      -v "$(pwd)"/certs:/certs \
      -e REGISTRY_HTTP_ADDR=0.0.0.0:443 \
      -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
      -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key \
      -p 443:443 \
      registry:2
    ```

4.  Docker clients can now pull from and push to your registry using its
    external address. The following commands demonstrate this:

    ```console
    $ docker pull ubuntu:16.04
    $ docker tag ubuntu:16.04 myregistry.domain.com/my-ubuntu
    $ docker push myregistry.domain.com/my-ubuntu
    $ docker pull myregistry.domain.com/my-ubuntu
    ```

#### Use an intermediate certificate

A certificate issuer may supply you with an *intermediate* certificate. In this
case, you must concatenate your certificate with the intermediate certificate to
form a *certificate bundle*. You can do this using the `cat` command:

```console
cat domain.crt intermediate-certificates.pem > certs/domain.crt
```

You can use the certificate bundle just as you use the `domain.crt` file in
the previous example.

### Support for Let's Encrypt

The registry supports using Let's Encrypt to automatically obtain a
browser-trusted certificate. For more information on Let's Encrypt, see
[https://letsencrypt.org/how-it-works/](https://letsencrypt.org/how-it-works/)
and the relevant section of the
[registry configuration](configuration.md#letsencrypt).

### Use an insecure registry (testing only)

It is possible to use a self-signed certificate, or to use our registry
insecurely. Unless you have set up verification for your self-signed
certificate, this is for testing only. See [run an insecure registry](insecure.md).

## Run the registry as a service

[Swarm services](../engine/swarm/services.md) provide several advantages over
standalone containers. They use a declarative model, which means that you define
the desired state and Docker works to keep your service in that state. Services
provide automatic load balancing scaling, and the ability to control the
distribution of your service, among other advantages. Services also allow you to
store sensitive data such as TLS certificates in
[secrets](../engine/swarm/secrets.md).

The storage back-end you use determines whether you use a fully scaled service
or a service with either only a single node or a node constraint.

- If you use a distributed storage driver, such as Amazon S3, you can use a
  fully replicated service. Each worker can write to the storage back-end
  without causing write conflicts.

- If you use a local bind mount or volume, each worker node writes to its
  own storage location, which means that each registry contains a different
  data set. You can solve this problem by using a single-replica service and a
  node constraint to ensure that only a single worker is writing to the bind
  mount.

The following example starts a registry as a single-replica service, which is
accessible on any swarm node on port 80. It assumes you are using the same
TLS certificates as in the previous examples.

First, save the TLS certificate and key as secrets:

```console
$ docker secret create domain.crt certs/domain.crt

$ docker secret create domain.key certs/domain.key
```

Next, add a label to the node where you want to run the registry.
To get the node's name, use `docker node ls`. Substitute your node's name for
`node1` below.

```console
$ docker node update --label-add registry=true node1
```

Next, create the service, granting it access to the two secrets and constraining
it to only run on nodes with the label `registry=true`. Besides the constraint,
you are also specifying that only a single replica should run at a time. The
example bind-mounts `/mnt/registry` on the swarm node to `/var/lib/registry/`
within the container. Bind mounts rely on the pre-existing source directory,
so be sure `/mnt/registry` exists on `node1`. You might need to create it before
running the following `docker service create` command.

By default, secrets are mounted into a service at `/run/secrets/<secret-name>`.

```console
$ docker service create \
  --name registry \
  --secret domain.crt \
  --secret domain.key \
  --constraint 'node.labels.registry==true' \
  --mount type=bind,src=/mnt/registry,dst=/var/lib/registry \
  -e REGISTRY_HTTP_ADDR=0.0.0.0:443 \
  -e REGISTRY_HTTP_TLS_CERTIFICATE=/run/secrets/domain.crt \
  -e REGISTRY_HTTP_TLS_KEY=/run/secrets/domain.key \
  --publish published=443,target=443 \
  --replicas 1 \
  registry:2
```

You can access the service on port 443 of any swarm node. Docker sends the
requests to the node which is running the service.

## Load balancing considerations

One may want to use a load balancer to distribute load, terminate TLS or
provide high availability. While a full load balancing setup is outside the
scope of this document, there are a few considerations that can make the process
smoother.

The most important aspect is that a load balanced cluster of registries must
share the same resources. For the current version of the registry, this means
the following must be the same:

  - Storage Driver
  - HTTP Secret
  - Redis Cache (if configured)

Differences in any of the above cause problems serving requests.
As an example, if you're using the filesystem driver, all registry instances
must have access to the same filesystem root, on
the same machine. For other drivers, such as S3 or Azure, they should be
accessing the same resource and share an identical configuration.
The _HTTP Secret_ coordinates uploads, so also must be the same across
instances. Configuring different redis instances works (at the time
of writing), but is not optimal if the instances are not shared, because
more requests are directed to the backend.

### Important/Required HTTP-Headers

Getting the headers correct is very important. For all responses to any
request under the "/v2/" url space, the `Docker-Distribution-API-Version`
header should be set to the value "registry/2.0", even for a 4xx response.
This header allows the docker engine to quickly resolve authentication realms
and fallback to version 1 registries, if necessary. Confirming this is setup
correctly can help avoid problems with fallback.

In the same train of thought, you must make sure you are properly sending the
`X-Forwarded-Proto`, `X-Forwarded-For`, and `Host` headers to their "client-side"
values. Failure to do so usually makes the registry issue redirects to internal
hostnames or downgrading from https to http.

A properly secured registry should return 401 when the "/v2/" endpoint is hit
without credentials. The response should include a `WWW-Authenticate`
challenge, providing guidance on how to authenticate, such as with basic auth
or a token service. If the load balancer has health checks, it is recommended
to configure it to consider a 401 response as healthy and any other as down.
This secures your registry by ensuring that configuration problems with
authentication don't accidentally expose an unprotected registry. If you're
using a less sophisticated load balancer, such as Amazon's Elastic Load
Balancer, that doesn't allow one to change the healthy response code, health
checks can be directed at "/", which always returns a `200 OK` response.

## Restricting access

Except for registries running on secure local networks, registries should always
implement access restrictions.

### Native basic auth

The simplest way to achieve access restriction is through basic authentication
(this is very similar to other web servers' basic authentication mechanism).
This example uses native basic authentication using `htpasswd` to store the
secrets.

> **Warning**:
> You **cannot** use authentication with authentication schemes that send
> credentials as clear text. You must
> [configure TLS first](deploying.md#run-an-externally-accessible-registry) for
> authentication to work.
{:.warning}

1.  Create a password file with one entry for the user `testuser`, with password
    `testpassword`:

    ```console
    $ mkdir auth
    $ docker run \
      --entrypoint htpasswd \
      httpd:2 -Bbn testuser testpassword > auth/htpasswd
    ```
    
    On Windows, make sure the output file is correctly encoded:

    ```powershell
    docker run --rm --entrypoint htpasswd httpd:2 -Bbn testuser testpassword | Set-Content -Encoding ASCII auth/htpasswd
    ```

2.  Stop the registry.

    ```console
    $ docker container stop registry
    ```

3.  Start the registry with basic authentication.

    ```console
    $ docker run -d \
      -p 5000:5000 \
      --restart=always \
      --name registry \
      -v "$(pwd)"/auth:/auth \
      -e "REGISTRY_AUTH=htpasswd" \
      -e "REGISTRY_AUTH_HTPASSWD_REALM=Registry Realm" \
      -e REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd \
      -v "$(pwd)"/certs:/certs \
      -e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
      -e REGISTRY_HTTP_TLS_KEY=/certs/domain.key \
      registry:2
      ```

4.  Try to pull an image from the registry, or push an image to the registry.
    These commands fail.

5.  Log in to the registry.

    ```console
    $ docker login myregistrydomain.com:5000
    ```

    Provide the username and password from the first step.

    Test that you can now pull an image from the registry or push an image to
    the registry.

> **X509 errors**: X509 errors usually indicate that you are attempting to use
> a self-signed certificate without configuring the Docker daemon correctly.
> See [run an insecure registry](insecure.md).

### More advanced authentication

You may want to leverage more advanced basic auth implementations by using a
proxy in front of the registry. See the [recipes list](recipes/index.md).

The registry also supports delegated authentication which redirects users to a
specific trusted token server. This approach is more complicated to set up, and
only makes sense if you need to fully configure ACLs and need more control over
the registry's integration into your global authorization and authentication
systems. Refer to the following [background information](spec/auth/token.md) and
[configuration information here](configuration.md#auth).

This approach requires you to implement your own authentication system or
leverage a third-party implementation.

## Deploy your registry using a Compose file

If your registry invocation is advanced, it may be easier to use a Docker
compose file to deploy it, rather than relying on a specific `docker run`
invocation. Use the following example `docker-compose.yml` as a template.

```yaml
registry:
  restart: always
  image: registry:2
  ports:
    - 5000:5000
  environment:
    REGISTRY_HTTP_TLS_CERTIFICATE: /certs/domain.crt
    REGISTRY_HTTP_TLS_KEY: /certs/domain.key
    REGISTRY_AUTH: htpasswd
    REGISTRY_AUTH_HTPASSWD_PATH: /auth/htpasswd
    REGISTRY_AUTH_HTPASSWD_REALM: Registry Realm
  volumes:
    - /path/data:/var/lib/registry
    - /path/certs:/certs
    - /path/auth:/auth
```

Replace `/path` with the directory which contains the `certs/` and `auth/`
directories.
{:.warning}

Start your registry by issuing the following command in the directory containing
the `docker-compose.yml` file:

```console
$ docker-compose up -d
```

## Considerations for air-gapped registries

You can run a registry in an environment with no internet connectivity.
However, if you rely on any images which are not local, you need to consider the
following:

- You may need to build your local registry's data volume on a connected
  host where you can run `docker pull` to get any images which are available
  remotely, and then migrate the registry's data volume to the air-gapped
  network.

- Certain images, such as the official Microsoft Windows base images, are not
  distributable. This means that when you push an image based on one of these
  images to your private registry, the non-distributable layers are **not**
  pushed, but are always fetched from their authorized location. This is fine
  for internet-connected hosts, but not in an air-gapped set-up.

  You can configure the Docker daemon to allow pushing non-distributable layers 
  to private registries.
  **This is only useful in air-gapped set-ups in the presence of
  non-distributable images, or in extremely bandwidth-limited situations.**
  You are responsible for ensuring that you are in compliance with the terms of
  use for non-distributable layers.

  1.  Edit the `daemon.json` file, which is located in `/etc/docker/` on Linux
      hosts and `C:\ProgramData\docker\config\daemon.json` on Windows Server.
      Assuming the file was previously empty, add the following contents:

      ```json
      {
        "allow-nondistributable-artifacts": ["myregistrydomain.com:5000"]
      }
      ```

      The value is an array of registry addresses, separated by commas.

      Save and exit the file.

  2.  Restart Docker.

  3.  Restart the registry if it does not start automatically.

  4.  When you push images to the registries in the list, their
      non-distributable layers are pushed to the registry.

      > **Warning**: Non-distributable artifacts typically have restrictions on
      > how and where they can be distributed and shared. Only use this feature
      > to push artifacts to private registries and ensure that you are in
      > compliance with any terms that cover redistributing non-distributable
      > artifacts.


## Next steps

More specific and advanced information is available in the following sections:

 - [Configuration reference](configuration.md)
 - [Working with notifications](notifications.md)
 - [Advanced "recipes"](recipes/index.md)
 - [Registry API](spec/api.md)
 - [Storage driver model](storage-drivers/index.md)
 - [Token authentication](spec/auth/token.md)
