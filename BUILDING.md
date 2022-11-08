
# Building the registry source

## Use-case

This is useful if you intend to actively work on the registry.

### Alternatives

Most people should use the [official Registry docker image](https://hub.docker.com/r/library/registry/).

People looking for advanced operational use cases might consider rolling their own image with a custom Dockerfile inheriting `FROM registry:2`.

OS X users who want to run natively can do so following [the instructions here](https://github.com/docker/docker.github.io/blob/master/registry/recipes/osx-setup-guide.md).

### Gotchas

You are expected to know your way around with go & git.

If you are a casual user with no development experience, and no preliminary knowledge of go, building from source is probably not a good solution for you.

## Configure the development environment

The first prerequisite of properly building distribution targets is to have a Go
development environment setup. Please follow [How to Write Go Code](https://golang.org/doc/code.html)
for proper setup. If done correctly, you should have a GOROOT and GOPATH set in the
environment.

Next, fetch the code from the repository using git:

    git clone https://github.com/distribution/distribution
    cd distribution

If you are planning to create a pull request with changes, you may want to clone directly from your [fork](https://help.github.com/en/articles/about-forks).

## Build and run from source

First, build the binaries:

    $ make
    + bin/registry
    + bin/digest
    + bin/registry-api-descriptor-template
    + binaries

Now create the directory for the registry data (this might require you to set permissions properly)

    mkdir -p /var/lib/registry

... or alternatively `export REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/somewhere` if you want to store data into another location.

The `registry`
binary can then be run with the following:

    $ ./bin/registry --version
    ./bin/registry github.com/distribution/distribution/v3 v2.7.0-1993-g8857a194

The registry can be run with a development config using the following
incantation:

    $ ./bin/registry serve cmd/registry/config-dev.yml
    INFO[0000] debug server listening :5001
    WARN[0000] No HTTP secret provided - generated random secret. This may cause problems with uploads if multiple registries are behind a load-balancer. To provide a shared secret, fill in http.secret in the configuration file or set the REGISTRY_HTTP_SECRET environment variable.  environment=development go.version=go1.18.3 instance.id=e837df62-a66c-4e04-a014-b063546e82e0 service=registry version=v2.7.0-1993-g8857a194
    INFO[0000] endpoint local-5003 disabled, skipping        environment=development go.version=go1.18.3 instance.id=e837df62-a66c-4e04-a014-b063546e82e0 service=registry version=v2.7.0-1993-g8857a194
    INFO[0000] endpoint local-8083 disabled, skipping        environment=development go.version=go1.18.3 instance.id=e837df62-a66c-4e04-a014-b063546e82e0 service=registry version=v2.7.0-1993-g8857a194
    INFO[0000] using inmemory blob descriptor cache          environment=development go.version=go1.18.3 instance.id=e837df62-a66c-4e04-a014-b063546e82e0 service=registry version=v2.7.0-1993-g8857a194
    INFO[0000] providing prometheus metrics on /metrics
    INFO[0000] listening on [::]:5000                        environment=development go.version=go1.18.3 instance.id=e837df62-a66c-4e04-a014-b063546e82e0 service=registry version=v2.7.0-1993-g8857a194

If it is working, one should see the above log messages.

### Build reference

The regular `go` commands, such as `go test`, should work per package.

A `Makefile` has been provided as a convenience to support repeatable builds.

Run `make` to build the binaries:

    $ make
    + bin/registry
    + bin/digest
    + bin/registry-api-descriptor-template
    + binaries

The above provides a repeatable build using the contents of the vendor
directory. We can verify this worked by running
the registry binary generated in the "./bin" directory:

    $ ./bin/registry --version
    ./bin/registry github.com/distribution/distribution v2.0.0-alpha.2-80-g16d8b2c.m

Run `make test` to run all of the tests.

Run `make validate` to run the validators, including the linter and vendor validation. You must have docker with the buildx plugin installed to run the validators.

### Optional build tags

Optional [build tags](http://golang.org/pkg/go/build/) can be provided using
the environment variable `DOCKER_BUILDTAGS`.

<dl>
<dt>noresumabledigest</dt>
<dd>Compiles without resumable digest support</dd>
<dt>include_gcs</dt>
<dd>Adds support for <a href="https://cloud.google.com/storage">Google Cloud Storage</a></dd>
<dt>include_oss</dt>
<dd>Adds support for <a href="https://www.alibabacloud.com/product/object-storage-service">Alibaba Cloud Object Storage Service (OSS)</a></dd>
</dl>
