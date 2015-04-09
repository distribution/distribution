# Docker Compose V1 + V2 registry

This compose configuration will setup a v1 and v2 registry behind an nginx
proxy. By default the combined registry may be accessed at localhost:5000.
This registry does not support pushing images to v2 and pull from v1. Clients
from before 1.6 will be configured to use the v1 registry, and newer clients
will use the v2 registry.

## Prerequisites
Install [docker-compose](https://github.com/docker/compose)

## How to run
```
$ docker-compose up
```

## How to push images
From a local project directory with Dockerfile
```
$ docker build -t localhost:5000/myimage .
$ docker push localhost:5000/myimage
```

