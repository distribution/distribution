> **Notice:** *This repository hosts experimental components that are
> currently under heavy and fast-paced development, not-ready for public
> consumption. If you are looking for the stable registry, please head over to
> [docker/docker-registry](https://github.com/docker/docker-registry)
> instead.*

Distribution
============

The Docker toolset to pack, ship, store, and deliver content.

The main product of this repository is the new registry implementation for
storing and distributing docker images. It supersedes the [docker/docker-
registry](https://github.com/docker/docker-registry) project with a new API
design, focused around security and performance.

The _Distribution_ project has the further long term goal of providing a
secure tool chain for distributing content. The specifications, APIs and tools
should be as useful with docker as they are without.

This repository contains the following components:

- **registry (beta):** An implementation of the [Docker Registry HTTP API
  V2](doc/spec/api.md) for use with docker 1.5+.
- **libraries (unstable):** A rich set of libraries for interacting with
  distribution components. Please see
  [godoc](http://godoc.org/github.com/docker/distribution) for details. Note
  that the libraries *are not* considered stable.
- **dist (experimental):** An experimental tool to provide distribution
  oriented functionality without the docker daemon.
- **specifications**: _Distribution_ related specifications are available in
  [doc/spec](doc/spec).
- **documentation:** Documentation is available in [doc](doc/overview.md).

### How will this integrate with Docker engine?

This project should provide an implementation to a V2 API for use in the
Docker core project. The API should be embeddable and simplify the process of
securely pulling and pushing content from docker daemons.

### What are the long term goals of the Distribution project?

Design a professional grade and extensible content distribution system, that
allow users to:

* Enjoy an efficient, secured and reliable way to store, manage, package and
  exchange content
* Hack/roll their own on top of healthy open-source components
* Implement their own home made solution through good specs, and solid
  extensions mechanism.

Features
--------

The new registry implementation provides the following benefits:

- faster push and pull
- new, more efficient implementation
- simplified deployment
- pluggable storage backend
- webhook notifications

Installation
------------

**TODO(stevvooe):** Add the following here:
- docker file
- binary builds for non-docker environment (test installations, etc.)

Configuration
-------------

The registry server can be configured with a yaml file. The following is a
simple example that can used for local development:

```yaml
version: 0.1
loglevel: debug
storage:
    filesystem:
        rootdirectory: /tmp/registry-dev
http:
    addr: localhost:5000
    secret: asecretforlocaldevelopment
    debug:
        addr: localhost:5001
```

The above configures the registry instance to run on port 5000, binding to
"localhost", with the debug server enabled. Registry data will be stored in
"/tmp/registry-dev". Logging will be in "debug" mode, which is the most
verbose.

A similar simple configuration is available at [cmd/registry/config.yml],
which is generally useful for local development.

**TODO(stevvooe): Need a "best practice" configuration overview. Perhaps, we
can point to a documentation section.

For full details about configuring a registry server, please see [the
documentation](doc/configuration.md).

### Upgrading

**TODO:** Add a section about upgrading from V1 registry along with link to
migrating in documentation.

Build
-----

If a go development environment is setup, one can use `go get` to install the
`registry` command from the current latest:

```sh
go get github.com/docker/distribution/cmd/registry
```

The above will install the source repository into the `GOPATH`. The `registry`
binary can then be run with the following:

```
$ $GOPATH/bin/registry -version
$GOPATH/bin/registry github.com/docker/distribution v2.0.0-alpha.1+unknown
```

The registry can be run with the default config using the following
incantantation:

```
$ $GOPATH/bin/registry $GOPATH/src/github.com/docker/distribution/cmd/registry/config.yml
INFO[0000] endpoint local-8082 disabled, skipping        app.id=34bbec38-a91a-494a-9a3f-b72f9010081f version=v2.0.0-alpha.1+unknown
INFO[0000] endpoint local-8083 disabled, skipping        app.id=34bbec38-a91a-494a-9a3f-b72f9010081f version=v2.0.0-alpha.1+unknown
INFO[0000] listening on :5000                            app.id=34bbec38-a91a-494a-9a3f-b72f9010081f version=v2.0.0-alpha.1+unknown
INFO[0000] debug server listening localhost:5001
```

If it is working, one should see the above log messages.

### Repeatable Builds

For the full development experience, one should `cd` into
`$GOPATH/src/github.com/docker/distribution`. From there, the regular `go`
commands, such as `go test`, should work per package (please see
[Developing](#developing) if they don't work).

A `Makefile` has been provided as a convenience to support repeatable builds.
Please install the following into `GOPATH` for it to work:

```
go get github.com/tools/godep github.com/golang/lint/golint
```

**TODO(stevvooe):** Add a `make setup` command to Makefile to run this. Have
to think about how to interact with Godeps properly.

Once these commands are available in the `GOPATH`, run `make` to get a full
build:

```
$ GOPATH=`godep path`:$GOPATH make
+ clean
+ fmt
+ vet
+ lint
+ build
github.com/docker/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar
github.com/Sirupsen/logrus
github.com/docker/libtrust
...
github.com/yvasiyarov/gorelic
github.com/docker/distribution/registry/handlers
github.com/docker/distribution/cmd/registry
+ test
...
ok    github.com/docker/distribution/digest 7.875s
ok    github.com/docker/distribution/manifest 0.028s
ok    github.com/docker/distribution/notifications  17.322s
?     github.com/docker/distribution/registry [no test files]
ok    github.com/docker/distribution/registry/api/v2  0.101s
?     github.com/docker/distribution/registry/auth  [no test files]
ok    github.com/docker/distribution/registry/auth/silly  0.011s
...
+ /Users/sday/go/src/github.com/docker/distribution/bin/registry
+ /Users/sday/go/src/github.com/docker/distribution/bin/registry-api-descriptor-template
+ /Users/sday/go/src/github.com/docker/distribution/bin/dist
+ binaries
```

The above provides a repeatable build using the contents of the vendored
Godeps directory. This includes formatting, vetting, linting, building,
testing and generating tagged binaries. We can verify this worked by running
the registry binary generated in the "./bin" directory:

```sh
$ ./bin/registry -version
./bin/registry github.com/docker/distribution v2.0.0-alpha.2-80-g16d8b2c.m
```

### Developing

The above approaches are helpful for small experimentation. If more complex
tasks are at hand, it is recommended to employ the full power of `godep`.

The Makefile is designed to have its `GOPATH` defined externally. This allows
one to experiment with various development environment setups. This is
primarily useful when testing upstream bugfixes, by modifying local code. This
can be demonstrated using `godep` to migrate the `GOPATH` to use the specified
dependencies. The `GOPATH` can be migrated to the current package versions
declared in `Godeps` with the following command:

```sh
godep restore
```

> **WARNING:** This command will checkout versions of the code specified in
> Godeps/Godeps.json, modifying the contents of `GOPATH`. If this is
> undesired, it is recommended to create a workspace devoted to work on the
> _Distribution_ project.

With a successful run of the above command, one can now use `make` without
specifying the `GOPATH`:

```sh
$ make
```

If that is successful, standard `go` commands, such as `go test` should work,
per package, without issue.

Support
-------

If any issues are encountered while using the _Distribution_ project, several
avenues are available for support:

IRC: #docker-distribution on FreeNode
Issue Tracker: github.com/docker/distribution/issues
Google Groups: https://groups.google.com/a/dockerproject.org/forum/#!forum/distribution
Mailing List: docker@dockerproject.org

Contribute
----------

Please see [CONTRIBUTING.md](CONTRIBUTING.md).

License
-------

This project is distributed under [Apache License, Version 2.0](LICENSE.md).
