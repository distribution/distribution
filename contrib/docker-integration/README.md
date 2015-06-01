# Docker Registry Integration Testing

These integration tests cover interactions between the Docker daemon and the
registry server. All tests are run using the docker cli.

The compose configuration is intended to setup a testing environment for Docker
using multiple registry configurations. These configurations include different
combinations of a v1 and v2 registry as well as TLS configurations.

## Running inside of Docker
### Get integration container
The container image to run the integation tests will need to be pulled or built
locally.

*Building locally*
```
$ docker build -t distribution/docker-integration .
```

### Run script

Invoke the tests within Docker through the `run.sh` script.

```
$ ./run.sh
```

Run with aufs driver and tmp volume
**NOTE: Using a volume will prevent multiple runs from needing to
re-pull images**
```
$ STORAGE_DRIVER=aufs DOCKER_VOLUME=/tmp/volume ./run.sh
```

## Running manually outside of Docker

### Install Docker Compose

[Docker Compose Installation Guide](http://docs.docker.com/compose/install/)

### Start compose setup
```
docker-compose up
```

### Install Certificates
The certificates must be installed in /etc/docker/cert.d in order to use TLS
client auth and use the CA certificate.
```
sudo sh ./install_certs.sh
```

### Test with Docker
Tag an image as with any other private registry. Attempt to push the image.

```
docker pull hello-world
docker tag hello-world localhost:5440/hello-world
docker push localhost:5440/hello-world

docker tag hello-world localhost:5441/hello-world
docker push localhost:5441/hello-world
# Perform login using user `testuser` and password `passpassword`
```

### Set /etc/hosts entry
Find the non-localhost ip address of local machine

### Run bats
Run the bats tests after updating /etc/hosts, installing the certificates, and
running the `docker-compose` script.
```
bats -p .
```

## Configurations

Port | V2 | V1 | TLS | Authentication
--- | --- | --- | --- | ---
5000 | yes | yes | no | none
5001 | no | yes | no | none
5002 | yes | no | no | none
5011 | no | yes | yes | none
5440 | yes | yes | yes | none
5441 | yes | yes | yes | basic (testuser/passpassword)
5442 | yes | yes | yes | TLS client
5443 | yes | yes | yes | TLS client (no CA)
5444 | yes | yes | yes | TLS client + basic (testuser/passpassword)
5445 | yes | yes | yes (no CA) | none
5446 | yes | yes | yes (no CA) | basic (testuser/passpassword)
5447 | yes | yes | yes (no CA) | TLS client
5448 | yes | yes | yes (SSLv3) | none
