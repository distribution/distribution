---
description: High-level overview of the Registry
keywords: registry, on-prem, images, tags, repository, distribution
redirect_from:
- /registry/overview/
title: Docker Registry
---

{% include registry.md %}

## What it is

The Registry is a stateless, highly scalable server side application that stores
and lets you distribute Docker images. The Registry is open-source, under the
permissive [Apache license](https://en.wikipedia.org/wiki/Apache_License).

## Why use it

You should use the Registry if you want to:

 * tightly control where your images are being stored
 * fully own your images distribution pipeline
 * integrate image storage and distribution tightly into your in-house development workflow

## Alternatives

Users looking for a zero maintenance, ready-to-go solution are encouraged to
head-over to the [Docker Hub](https://hub.docker.com), which provides a
free-to-use, hosted Registry, plus additional features (organization accounts,
automated builds, and more).

## Requirements

The Registry is compatible with Docker engine **version 1.6.0 or higher**.

## Basic commands

Start your registry

    docker run -d -p 5000:5000 --name registry registry:2

Pull (or build) some image from the hub

    docker pull ubuntu

Tag the image so that it points to your registry

    docker image tag ubuntu localhost:5000/myfirstimage

Push it

    docker push localhost:5000/myfirstimage

Pull it back

    docker pull localhost:5000/myfirstimage

Now stop your registry and remove all data

    docker container stop registry && docker container rm -v registry

## Next

You should now read the [detailed introduction about the registry](introduction.md),
or jump directly to [deployment instructions](deploying.md).
