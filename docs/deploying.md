<!--GITHUB
page_title: Deploying a registry server
page_description: Explains how to deploy a registry server
page_keywords: registry, service, images, repository
IGNORES-->


# Deploying a registry server

You obviously need to [install Docker](https://docs.docker.com/installation/) (remember you need Docker version 1.6.0 or newer).

## Getting started in 2 lines

Create a folder for your registry data:

     $ mkdir registry-data

Start your registry:

     $ docker run -d -p 5000:5000 \
     		-v `pwd`/registry-data:/tmp/registry-dev \
     		--restart=always --name registry registry:2

That's it.

You can now tag an image and push it:

    $ docker tag ubuntu localhost:5000/batman/ubuntu
    $ docker push localhost:5000/batman/ubuntu

Then pull it:

    $ docker pull localhost:5000/batman/ubuntu


## Making your Registry available

Now that your registry works on localhost, you probably want to make it available as well to other hosts.

Let assume your registry is accessible via the domain name `myregistrydomain.com` (still on port `5000`).

If you try to `docker pull myregistrydomain.com:5000/batman/ubuntu`, you will see the following error message:

```
FATA[0000] Error response from daemon: v1 ping attempt failed with error: Get
https://nonregistry:5000/v1/_ping: dial tcp: lookup nonregistry: no such host. If
this private registry supports only HTTP or HTTPS with an unknown CA certificate,
please add `--insecure-registry nonregistry:5000` to the daemon's arguments. In
the case of HTTPS, if you have access to the registry's CA certificate, no need
for the flag; simply place the CA certificate at /etc/docker/certs.d/nonregistry:5000/ca.crt
```

You basically have three different options to comply with docker security requirements here.

### 1. buy a SSL certificate for your domain

This is the (highly) recommended solution.

You can buy a certificate for as cheap as 10$ a year (some registrars even offer certificates for free), and this will save you a lot of trouble.

Assuming you now have a `domain.crt` and `domain.key` inside a directory named `certs`:

```
# Stop your registry
docker stop registry && docker rm registry

# Start your registry with TLS enabled
docker run -d -p 5000:5000 \
	-v `pwd`/registry-data:/tmp/registry-dev \
	-v `pwd`/certs:/certs \
	-e REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt \
	-e REGISTRY_HTTP_TLS_KEY=/certs/domain.key \
	--restart=always --name registry \
	registry:2
```

**Pros:**

 - best solution
 - work without further ado (assuming you bought your certificate from a CA that is trusted by your operating system)

**Cons:**

 - ?

### 2. instruct docker to trust your registry as insecure

This basically tells Docker to entirely disregard security for your registry.

1. edit the file `/etc/default/docker` so that there is a line that reads: `DOCKER_OPTS="--insecure-registry myregistrydomain:5000"` (or add that to existing `DOCKER_OPTS`)
2. restart your Docker daemon: on ubuntu, this is usually `service docker stop && service docker start`

**Pros:**

 - easy to configure
 
**Cons:**
 
 - very insecure
 - you have to configure every docker daemon that wants to access your registry 
  
### 3. use a self-signed certificate and configure docker to trust it

Alternatively, you can generate your own certificate:

```
mkdir -p certs && openssl req \
	-newkey rsa:4096 -nodes -sha256 -keyout certs/domain.key \
	-x509 -days 365 -out certs/domain.crt
```

Be sure to use the name `myregistrydomain.com` as a CN.

Now go to solution 1 above and stop and restart your registry.

Then you have to instruct every docker daemon to trust that certificate. This is done by copying the `domain.crt` file to `/etc/docker/certs.d/myregistrydomain.com:5000/ca.crt`

**Pros:**

 - more secure than solution 2

**Cons:**

 - you have to configure every docker daemon that wants to access your registry



## Using Compose

It's highly recommended to use Docker Compose to facilitate managing your Registry configuration.

Here is a simple `docker-compose.yml` that does setup your registry exactly as above, with TLS enabled.

```
registry:
  restart: always
  image: registry:2
  ports:
    - 5000:5000
  environment:
    REGISTRY_HTTP_TLS_CERTIFICATE: /certs/domain.crt
    REGISTRY_HTTP_TLS_KEY: /certs/domain.key
    REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY: /data
  volumes:
    - /path/registry-data:/data
    - /path/certs:/certs
```

You can then start your registry with a simple

    $ docker-compose up -d


## Next

You are now ready to explore [the registry configuration](configuration.md)
