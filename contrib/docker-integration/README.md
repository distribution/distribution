# Docker Registry Multi-Configuration Testing

This compose configuration is intended to setup a testing environment for Docker
using multiple registry configurations. These configurations include different
combinations of a v1 and v2 registry as well as TLS configurations.

### Limitations

Currently this setup is configured to use localhost as the hostname which
limits the ease of testing within Docker since localhost is always treated
as an insecure registry. To treat localhost as secure the Docker code must
be modified. Without localhost as secure, the test cases will not distinguish
between a TLS configuration with a CA and self-signed.

### Install Docker Compose

1. Open a new terminal on the host with your `distribution` source.

2. Get the `docker-compose` binary.

		$ sudo wget https://github.com/docker/compose/releases/download/1.1.0/docker-compose-`uname  -s`-`uname -m` -O /usr/local/bin/docker-compose

	This command installs the binary in the `/usr/local/bin` directory.

3. Add executable permissions to the binary.

		$  sudo chmod +x /usr/local/bin/docker-compose

## Usage

### Start compose setup
```
docker-compose up
```

### Install Certificates
The certificates must be installed in /etc/docker/cert.d in order to use TLS client auth and use the CA certificate.
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

## Configurations

Port | V2 | V1 | TLS | Authentication
--- | --- | --- | --- | ---
5000 | yes | yes | no | none
5001 | no | yes | no | none
5002 | yes | no | no | none
5440 | yes | yes | yes | none
5441 | yes | yes | yes | basic (testuser/passpassword)
5442 | yes | yes | yes | TLS client
5443 | yes | yes | yes | TLS client (no CA)
5444 | yes | yes | yes | TLS client + basic (testuser/passpassword)
5445 | yes | yes | yes (no CA) | none
5446 | yes | yes | yes (no CA) | basic (testuser/passpassword)
5447 | yes | yes | yes (no CA) | TLS client
5448 | yes | yes | yes (SSLv3) | none
