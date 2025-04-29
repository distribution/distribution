## 2DFS+OCI distribution extension

This is a fork of the [OCI distribution](https://github.com/distribution/distribution) project, which is a core component of the OCI container ecosystem.

[![Build Status](https://github.com/2DFS/2dfs-registry/workflows/build/badge.svg?branch=main&event=push)](https://github.com/2DFS/2dfs-registry/actions/workflows/build.yml?query=workflow%3Abuild)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![OCI Conformance](https://github.com/2DFS/2dfs-registry/workflows/conformance/badge.svg)](https://github.com/2DFS/2dfs-registry/actions?query=workflow%3Aconformance)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/2DFS/2dfs-registry/badge)](https://securityscorecards.dev/viewer/?uri=github.com/2DFS/2dfs-registry)


Based on the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec).
This Fork provides additional support for `2dfs.field` layer type, layer flattening for OCI compatibility and semantic partitioning.  

This repository contains the following components:

|**Component**       |Description                                                                                                                                                                                         |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **registry**       | An implementation of the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec).                                                                                                 |
| **libraries**      | A rich set of libraries for interacting with distribution components. Please see [godoc](https://pkg.go.dev/github.com/2DFS/2dfs-registry) for details. **Note**: The interfaces for these libraries are **unstable**. |
| **documentation**  | Full documentation is available at [https://distribution.github.io/distribution](https://distribution.github.io/distribution/).

### Getting Started 

- Build the registry using `sudo docker build -t registry .`

- Run the registry using `sudo docker run -d -p 5000:5000 --restart=always --name registry registry`

### How does this integrate with Docker, containerd, and other OCI client?

Clients implement against the OCI specification and communicate with the
registry using HTTP. 
This project implements *semantic tags* allowing on demand image partitioning for **2DFS** compliant images. 

## Contribution

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to contribute
issues, fixes, and patches to this project. If you are contributing code, see
the instructions for [building a development environment](BUILDING.md).

## Licenses

The distribution codebase is released under the [Apache 2.0 license](LICENSE).
The README.md file, and files in the "docs" folder are licensed under the
Creative Commons Attribution 4.0 International License. You may obtain a
copy of the license, titled CC-BY-4.0, at http://creativecommons.org/licenses/by/4.0/.
