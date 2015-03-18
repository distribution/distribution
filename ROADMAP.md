# Roadmap

This document covers the high-level the goals and dates for features in the
docker registry. The distribution project currently has several components to
report in the road map, which are covered below.

## Goals

- Replace the existing [docker registry](github.com/docker/docker-registry)
  implementation as the primary implementation.
- Replace the existing push and pull code in the docker engine with the
  distribution package.
- Define a strong data model for distributing docker images
- Provide a flexible distribution tool kit for use in the docker platform

## Components

The distribution project has a few components with independent road maps and
road maps related to the docker project. They are covered below.

### Registry

The current status of the registry road map is managed via github
[milestones](https://github.com/docker/distribution/milestones). Upcoming
features and bugfixes will be added to relevant milestones. If a feature or
bugfix is not part of a milestone, it is currently unscheduled for
implementation.

The high-level goals for each registry release are part of this section.

#### 2.0

Milestones: [2.0.0-beta](https://github.com/docker/distribution/milestones/Registry/2.0.0-beta) [2.0.0-rc](https://github.com/docker/distribution/milestones/Registry/2.0.0-rc) [2.0.0](https://github.com/docker/distribution/milestones/Registry/2.0.0)

The 2.0 release is the first release of the new registry. This is mostly
focused on implementing the [new registry
API](https://github.com/docker/distribution/blob/master/doc/spec/api.md) with
a focus on security and performance.

Features:

- Faster push and pull
- New, more efficient implementation
- Simplified deployment
- Full API specification for V2 protocol
- Pluggable storage system (s3, azure, filesystem and inmemory supported)
- Immutable manifest references ([#46](https://github.com/docker/distribution/issues/46))
- Webhook notification system ([#42](https://github.com/docker/distribution/issues/42))
- Native TLS Support ([#132](https://github.com/docker/distribution/pull/132))
- Pluggable authentication system
- Health Checks ([#230](https://github.com/docker/distribution/pull/230))

#### 2.1

Milestone: [2.1](https://github.com/docker/distribution/milestones/Registry/2.1)

Planned Features:

> **NOTE:** This feature list is incomplete at this time.

- Support for Manifest V2, Schema 2 and explicit tagging objects ([#62](https://github.com/docker/distribution/issues/62), [#173](https://github.com/docker/distribution/issues/173))
- Mirroring ([#19](https://github.com/docker/distribution/issues/19))
- Flexible client package based on distribution interfaces ([#193](https://github.com/docker/distribution/issues/193)

#### 2.2

Milestone: [2.2](https://github.com/docker/distribution/milestones/Registry/2.2)

TBD

### Docker Platform

To track various requirements that must be synced with releases of the docker
platform, we've defined labels corresponding to upcoming releases. Each
release also has a project page explaining the relationship of the
distribution project with the docker project.

Please see the following table for more information:

| Platform Version | Milestone | Project |
|-----------|------|-----|
| Docker 1.6 |  [Docker/1.6](https://github.com/docker/distribution/labels/docker%2F1.6) | [Project](https://github.com/docker/distribution/wiki/docker-1.6-Project-Page) |
| Docker 1.7|  [Docker/1.7](https://github.com/docker/distribution/labels/docker%2F1.7) | [Project](https://github.com/docker/distribution/wiki/docker-1.7-Project-Page) |
| Docker 1.8|  [Docker/1.8](https://github.com/docker/distribution/labels/docker%2F1.8) | [Project](https://github.com/docker/distribution/wiki/docker-1.8-Project-Page) |

### Package

The distribution project, at its core, is a set of Go packages that make up
distribution components. At this time, most of these packages make up the
registry implementation. The package itself is considered unstable. If you're
using it, please take care to vendor the dependent version. For feature
additions, please see the Registry section. In the future, we may break out a
separate road map for distribution specific features that apply to more than
just the registry.