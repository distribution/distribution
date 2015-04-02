page_title: Docker Registry Service 2.0
page_description: Introduces the docker registry service
page_keywords: registry, service, images, repository

# Docker Registry Service 2.0

The Docker Registry Service stores and distributes Docker images. The majority
of Docker users pull images from Docker's own public registry instance.
Installing Docker gives users this ability. Users with a Docker Hub account can
also push images to this registry.

A subset of Docker users may wish to deploy a Docker Registry Service of their own. For example, users with their own software products and may want to maintain an image store for private, company use. Some companies also maintain a  registry instance for release of their software images to the public. 

This documentation introduces the registry for users deploying their own instances. You can use this documentation to understand how to configure capabilities into a registry instance or how to write your own custom software to extend the existing service.


## Understanding the registry service

A registry is, at its heart, a collection of repositories. In turn, a repository is collection of images. Users interact with the registry by pushing images to or pulling images from the registry. The Docker Registry Service includes several optional features that you can configure according to your needs.

![](/distribution/images/registry.png)

The architecture supports a configurable storage backend. You can store images on  a file system or on a service such as Amazon S3 or Microsoft Azure. The default storage system is the local disk; this is suitable for development or some small deployments.

Securing access to images is a concern for even the simplest deployment. The registry service supports transport layer security (TLS) natively. You must configure it in your instance to make use of it. You can also use a proxy server such as Nginx and basic authentication to extend the security of a deployment.  

The registry repository includes reference implementations for additional authentication and authorization support. Only very large or public registry deployments are expected to extend the registry in this way.

Docker Registry Service architecture includes a robust notification system. This system sends webhook notifications in response to registry activity.  The registry also includes features for both logging and reporting as well. Reporting is useful for large installations that want to collect metrics. Currently, the feature supports both New Relic and Bugsnag.


## Support

If any issues are encountered while using the _Distribution_ project, several
avenues are available for support:

<table>
<tr>
	<th align="left">
	IRC
	</th>
	<td>
	#docker-distribution on FreeNode
	</td>
</tr>
<tr>
	<th align="left">
	Issue Tracker
	</th>
	<td>
	github.com/docker/distribution/issues
	</td>
</tr>
<tr>
	<th align="left">
	Google Groups
	</th>
	<td>
	https://groups.google.com/a/dockerproject.org/forum/#!forum/distribution
	</td>
</tr>
<tr>
	<th align="left">
	Mailing List
	</th>
	<td>
	docker@dockerproject.org
	</td>
</tr>
</table>