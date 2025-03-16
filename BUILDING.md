
# Building the registry source

## Use-case

This is useful if you intend to actively work on the registry.

### Alternatives

Most people should use prebuilt images, for example, the [Registry docker image](https://hub.docker.com/r/library/registry/) provided by Docker.

People looking for advanced operational use cases might consider rolling their own image with a custom Dockerfile inheriting `FROM registry:2`.

The latest updates to `main` branch are automatically pushed to [distribution Docker Hub repository](https://hub.docker.com/r/distribution/distribution) and tagged with `edge` tag.

### Gotchas

You are expected to know your way around with `go` & `git`.

If you are a casual user with no development experience, and no preliminary knowledge of Go, building from source is probably not a good solution for you.

## Configure the development environment

The first prerequisite of properly building distribution targets is to have a Go
development environment setup. Please follow [How to Write Go Code](https://go.dev/doc/code) for proper setup.

Next, fetch the code from the repository using git:

    git clone https://github.com/distribution/distribution
    cd distribution

If you are planning to create a pull request with changes, you may want to clone directly from your [fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/about-forks).

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
the environment variable `BUILDTAGS`.

<dl>
<dt>noresumabledigest</dt>
<dd>Compiles without resumable digest support</dd>
</dl>

### Local S3 store environment

You can run an S3 API compatible store locally with [minio](https://min.io/).

You must have a [docker compose](https://docs.docker.com/compose/) compatible tool installed on your workstation.

Start the local S3 store environment:
```
make start-s3-storage
```
There is a sample registry configuration file that lets you point the registry to the started storage:
```
AWS_ACCESS_KEY=distribution \
        AWS_SECRET_KEY=password \
        AWS_REGION=us-east-1 \
        S3_BUCKET=images-local \
        S3_ENCRYPT=false \
        REGION_ENDPOINT=http://127.0.0.1:9000 \
        S3_SECURE=false \
./bin/registry serve tests/conf-local-s3.yml
```
Stop the local S3 store when done:
```
make stop-s3-storage
```
