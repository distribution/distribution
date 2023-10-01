# Distribution

This repository provides container images for the Open Source Registry implementation for storing and distributing container artifacts in conformance with the
[OCI Distribution Specification](https://github.com/opencontainers/distribution-spec).

<img src="https://raw.githubusercontent.com/distribution/distribution/main/distribution-logo.svg" width="200px" />

[![Build Status](https://github.com/distribution/distribution/workflows/CI/badge.svg?branch=main&event=push)](https://github.com/distribution/distribution/actions?query=workflow%3ACI)
[![OCI Conformance](https://github.com/distribution/distribution/workflows/conformance/badge.svg)](https://github.com/distribution/distribution/actions?query=workflow%3Aconformance)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)

## Quick start

Run the registry locally with the [default configuration](https://github.com/distribution/distribution/blob/main/cmd/registry/config-dev.yml):
```
docker run -d -p 5000:5000 --restart always --name registry distribution/distribution:edge
```

*NOTE:* in order to run push/pull against the locally run registry you must allow
your docker (containerd) engine to use _insecure_ registry by editing `/etc/docker/daemon.json` and subsequently restarting it
```
{
     "insecure-registries": ["host.docker.internal:5000"]
}
```

Now you are ready to use it:
```
docker pull alpine
docker tag alpine localhost:5000/alpine
docker push localhost:5000/alpine
```

⚠️  Beware the default configuration uses [`filesystem` storage driver](https://github.com/distribution/distribution/blob/main/docs/storage-drivers/filesystem.md)
and the above example command does not mount a local filesystem volume into the running container.
If you wish to mount the local filesystem to the `rootdirectory` of the
`filesystem` storage driver run the following command:
```
docker run -d -p 5000:5000 $PWD/FS/PATH:/var/lib/registry --restart always --name registry distribution/distribution:edge
```

### Custom configuration

If you don't wan to use the default configuration file, you can supply
your own custom configuration file as follows:
```
docker run -d -p 5000:5000 $PWD/PATH/TO/config.yml:/etc/docker/registry/config.yml --restart always --name registry distribution/distribution:edge
```

## Communication

For async communication and long-running discussions please use issues and pull requests
on the [GitHub repo](https://github.com/distribution/distribution).

For sync communication we have a #distribution channel in the [CNCF Slack](https://slack.cncf.io/)
that everyone is welcome to join and chat about development.
