---
description: High-level overview of the Registry
keywords: registry, on-prem, images, tags, repository, distribution
title: Distribution Registry
---

## What it is

The Registry is a stateless, highly scalable server side application that stores
and lets you distribute container images and other content. The Registry is open-source, under the
permissive [Apache license](https://en.wikipedia.org/wiki/Apache_License).

## Why use it

You should use the Registry if you want to:

 * tightly control where your images are being stored
 * fully own your images distribution pipeline
 * integrate image storage and distribution tightly into your in-house development workflow

## Alternatives

Users looking for a zero maintenance, ready-to-go solution are encouraged to
use one of the existing registry services. Many of these provide support and security
scanning, and are free for public repositories. For example:
- [Docker Hub](https://hub.docker.com)
- [Quay.io](https://quay.io/)
- [GitHub Packages](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)

Cloud infrastructure providers such as [AWS](https://aws.amazon.com/ecr/), [Azure](https://azure.microsoft.com/products/container-registry/), [Google Cloud](https://cloud.google.com/artifact-registry) and [IBM Cloud](https://www.ibm.com/products/container-registry) also have container registry services available at a cost.

## Compatibility

The distribution registry implements the [OCI Distribution Spec](https://github.com/opencontainers/distribution-spec) version 1.0.1.

## Basic commands

Start your registry

```sh
docker run -d -p 5000:5000 --name registry registry:2
```

Pull (or build) some image from the hub

```sh
docker pull ubuntu
```

Tag the image so that it points to your registry

```sh
docker image tag ubuntu localhost:5000/myfirstimage
```

Push it

```sh
docker push localhost:5000/myfirstimage
```

Pull it back

```sh
docker pull localhost:5000/myfirstimage
```

Now stop your registry and remove all data

```sh
docker container stop registry && docker container rm -v registry
```

## Next

You should now read the [detailed introduction about the registry](about),
or jump directly to [deployment instructions](about/deploying).
