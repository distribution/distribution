<!--GITHUB
page_title: Docker Registry 2.0
page_description: Introduces the Docker Registry
page_keywords: registry, images, repository
IGNORES-->

# Docker Registry

The Registry is a stateless, highly scalable server side component that stores and lets you distribute Docker images.

Users looking for a ready-to-go solution are encouraged to head-over to the [Docker Hub](https://hub.docker.com), which provides a free-to-use, hosted Registry, plus additional features (organization accounts, automated builds, and more).

On the other hand, people interested in finer grained integration, or more control over the content they publish, should run and operate their own Registry.

## About versions

You need to use a Docker client that is version 1.6.0 or newer for this to work.
If you really need to work with older Docker versions, you should look into the [old python registry](https://github.com/docker/docker-registry)

## Understanding the Registry

A registry is a storage and content delivery system, holding named Docker images, available in different tagged versions. For example, the image `distribution/registry`, with tags `2.0` and `latest`.

Users interact with a registry by using docker push and pull commands. For example, `docker pull myregistry.com/stevvooe/batman:voice`.

Storage itself is delegated to drivers. The default storage driver is the local posix filesystem, which is suitable for development or small deployments. Additional cloud-based storage driver like S3, Microsoft Azure and Ceph are also supported. People looking into using other storage backends may do so by writing their own driver implementing the [Storage API](storagedrivers.md).

Since securing access to your hosted images is paramount, the Registry natively supports TLS. You can also enforce basic authentication through a proxy like Nginx.

The Registry GitHub repository includes reference implementations for additional authentication and authorization methods. Only very large or public deployments are expected to extend the Registry in this way.

Finally, the Registry includes a robust [notification system](notifications.md), calling webhooks in response to activity, and both extensive logging and reporting. Reporting is mostly useful for large installations that want to collect metrics. Currently, New Relic and Bugsnag are supported.

## Getting help

The Registry is an open source project and is under active development. If you need help, would like to contribute, or simply want to talk about the project, we have a number of open channels for communication.

- To report bugs or file feature requests: please use the [issue tracker on Github](https://github.com/docker/distribution/issues).
- To talk about the project please post a message to the [mailing list](https://groups.google.com/a/dockerproject.org/forum/#!forum/distribution) or join the `#docker-distribution` channel on IRC.
- To contribute code or documentation changes: please submit a [pull request on Github](https://github.com/docker/distribution/pulls).

For more information and resources, please visit the [Getting Help project page](https://docs.docker.com/project/get-help/).

## Documentation

Basics:

 - [Deployment](deploying.md)
 - [Configuration](configuration.md)

Advanced:

 - [Storage driver model](storagedrivers.md)
 - [Working with notifications](notifications.md)
 - [Registry API](spec/api.md)