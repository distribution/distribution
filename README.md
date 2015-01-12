> **Notice:** *This repository hosts experimental components that are currently under heavy and fast-paced development, not-ready for public consumption. If you are looking for the stable registry, please head over to [docker/docker-registry](https://github.com/docker/docker-registry) instead.*

Distribution
============

The Docker toolset to pack, ship, store, and deliver content.

Planned content for this repository:

* Distribution related specifications
    - Image format
    - JSON registry API
* Registry implementation: a Golang implementation of the JSON API
* Client libraries to consume conforming implementations of the JSON API

# Ongoing open sprint

### What is an open sprint?

The open sprint is a focused effort of a small group of people to kick-off a new project, while commiting to becoming maintainers of the resulting work.

**Having a dedicated team work on the subject doesn't mean that you, the community, cannot contribute!** We need your input to make the best use of the sprint, and focus our work on what matters for you. For this particular topic:

* Come discuss on IRC: #docker-distribution on FreeNode
* Come read and participate in the [Google Groups](https://groups.google.com/a/dockerproject.org/forum/#!forum/distribution)
* Submit your ideas, and upvote those you think matter the most on [Google Moderator](https://www.google.com/moderator/?authuser=1#16/e=2165c3)

### Goal of the distribution sprint

Design a professional grade and extensible content distribution system, that allow users to:

* Enjoy an efficient, secured and reliable way to store, manage, package and exchange content
* Hack/roll their own on top of healthy open-source components
* Implement their own home made solution through good specs, and solid extensions mechanism.

### Schedule and expected output

The Open Sprint will start on **Monday December 29th**, and end on **Friday January 16th**.

What we want to achieve as a result is:

* Tactical fixes of today's frustrations in the existing Docker codebase
  - This includes a throrough review of [docker/docker#9784](https://github.com/docker/docker/pull/9784) by core maintainers

* Laying the base of a new distribution subsystem, living independently, and with a well defined group of maintainers. This is the purpose of this repository, which aims at hosting:
  - A specification of the v2 image format
  - A specification of the JSON/HTTP protocol
  - Server-side Go implementation of the v2 registry
  - Client-side Go packages to consume this new API
  - Standalone binaries providing content distribution functionalities outside of Docker

* Constraints for interface provided by Distribution to Core:
  - The distribution repository is a collection of tools for packaging and
    shipping content with Docker
  - All tools are usable primarily as standalone command-line tools. They may
    also expose bindings in one or more programming languages. Typically the
    first available language is Go, but that is not automatically true and more
    languages will be supported over time
  - The distribution repository is still under incubation, any code layout,
    interface and name may change before it gets included in a stable release of
    Docker

### How will this integrate with Docker engine?

Building awesome, independent, and well maintained distribution tools should give Docker core maintainers enough incentive to switch to the newly develop subsystem. We make no assumptions on a given date or milestone as urgency should be fixed through [docker/docker#9784](https://github.com/docker/docker/pull/9784), and in order to maintain focus on producing a top quality alternative.

### Relevant documents

* [Analysis of current state and goals](doc/opensprint/kickoff.md)
