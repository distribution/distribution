# Glossary

This page contains distribution related terms. For a complete Docker glossary,
see the [glossary in the full documentation set](http://docs.docker.com/reference/glossary/).

<dl>
	<dt>Blob</dt>
	<dd>
	The primary unit of registry storage. A string of bytes identified by
	content-address, known as a _digest_.
	</dd>

	<dt>Image</dt>
	<dd>An image is a collection of content from which a docker container can be created.</dd>

	<dt>Layer</dt>
	<dd>
	A tar file representing the partial content of a filesystem. Several
	layers can be "stacked" to make up the root filesystem.
	</dd>

	<dt>Manifest</dt>
	<dd>Describes a collection layers that make up an image.</dd>

	<dt>Registry</dt>
	<dd>A registry is a service which serves repositories.</dd>

	<dt>Repository</dt>
	<dd>
	A repository is a collection of docker images, made up of manifests, tags
	and layers. The base unit of these components are blobs.
	</dd>

	<dt>Tag</dt>
	<dd>Tag provides a common name to an image.</dd>

	<dt>Namespace</dt>
	<dd>A namespace is a collection of repositories with a common name prefix. The
	namespace with an empty common prefix is considered the Global Namespace.</dd>

	<dt>Scope</dt>
	<dd>A common repository name prefix.</dd>
</dl>
