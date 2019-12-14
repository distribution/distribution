# Distribution

The Docker toolset to pack, ship, store, and deliver content.

This repository's main product is the Open Source Docker Registry implementation
for storing and distributing Docker and OCI images using the
[OCI Distribution Specification](https://github.com/opencontainers/distribution-spec).
The goal of this project is to provide a simple, secure, and scalable base
for building a registry solution or running a simple private registry.

<img src="https://www.docker.com/sites/default/files/oyster-registry-3.png" width=200px/>

[![Build Status](https://travis-ci.org/docker/distribution.svg?branch=master)](https://travis-ci.org/docker/distribution)
[![GoDoc](https://godoc.org/github.com/docker/distribution?status.svg)](https://godoc.org/github.com/docker/distribution)

This repository contains the following components:

|**Component**       |Description                                                                                                                                                                                         |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **registry**       | An implementation of the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec).                                                                                                 |
| **libraries**      | A rich set of libraries for interacting with distribution components. Please see [godoc](https://godoc.org/github.com/docker/distribution) for details. **Note**: The interfaces for these libraries are **unstable**. |
| **documentation**  | Docker's full documentation set is available at [docs.docker.com](https://docs.docker.com). This repository [contains the subset](docs/) related just to the registry.                                                                                                                                          |

### How does this integrate with Docker, containerd, and other OCI client?

Clients implement against the OCI specification and communicate with the
registry using HTTP. This project contains an client implementation which
is currently in use by Docker, however, it is deprecated for the
[implementation in containerd](https://github.com/containerd/containerd/tree/master/remotes/docker)
and will not support new features.

### What are the long term goals of the Distribution project?

The _Distribution_ project has the further long term goal of providing a
secure tool chain for distributing content. The specifications, APIs and tools
should be as useful with Docker as they are without.

Our goal is to design a professional grade and extensible content distribution
system that allow users to:

* Enjoy an efficient, secured and reliable way to store, manage, package and
  exchange content
* Hack/roll their own on top of healthy open-source components
* Implement their own home made solution through good specs, and solid
  extensions mechanism.

### Who needs to deploy a registry?

By default, Docker users pull images from Docker's public registry instance.
[Installing Docker](https://docs.docker.com/engine/installation/) gives users this
ability. Users can also push images to a repository on Docker's public registry,
if they have a [Docker Hub](https://hub.docker.com/) account.

For some users and even companies, this default behavior is sufficient. For
others, it is not.

For example, users with their own software products may want to maintain a
registry for private, company images. Also, you may wish to deploy your own
image repository for images used to test or in continuous integration. For these
use cases and others, [deploying your own registry instance](https://github.com/docker/docker.github.io/blob/master/registry/deploying.md)
may be the better choice.

### Migration to Registry 2.0

For those who have previously deployed their own registry based on the Registry
1.0 implementation and wish to deploy a Registry 2.0 while retaining images,
data migration is required. A tool to assist with migration efforts has been
created. For more information see [docker/migrator](https://github.com/docker/migrator).

## Contribution

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to contribute
issues, fixes, and patches to this project. If you are contributing code, see
the instructions for [building a development environment](BUILDING.md).

## Communication

For async communication and long running discussions please use issues and pull requests on the github repo.
This will be the best place to discuss design and implementation.

For sync communication we have a community slack with a #distribution channel that everyone is welcome to join and chat about development.

**Slack:** Catch us in the #distribution channels on dockercommunity.slack.com.
[Click here for an invite to Docker community slack.](https://dockr.ly/slack)

## Licenses

The distribution codebase is released under the [Apache 2.0 license](LICENSE).
The README.md file, and files in the "docs" folder are licensed under the
Creative Commons Attribution 4.0 International License. You may obtain a
copy of the license, titled CC-BY-4.0, at http://creativecommons.org/licenses/by/4.0/.
