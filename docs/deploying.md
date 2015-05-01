<!--GITHUB
page_title: Deploying a registry server
page_description: Explains how to deploy a registry server
page_keywords: registry, service, images, repository
IGNORES-->


# Deploying a registry server

This section explains how to deploy a Docker Registry either privately
for your own company or publicly for other users. For example, your company may
require a private registry to support your continuous integration (CI) system as
it builds new releases or test servers. Alternatively, your company may have a
large number of products or services with images you wish to serve in a branded
manner.

Docker's public registry maintains a default `registry` image to assist you in the
deployment process. This registry image is sufficient for running local tests
but is insufficient for production. For production you should configure and
build your own custom registry image from the `docker/distribution` code.

>**Note**: The examples on this page were written and tested using Ubuntu 14.04. 
>If you are running Docker in a different OS, you may need to "translate"
>the commands to meet the requirements of your own environment. 


## Simple example with the official image

In this section, you create a container running Docker's official registry
image. You push an image to, and then pull the same image from, this registry.
This a good exercise for understanding the basic interactions a client has with
a local registry.

1. Install Docker.

2. Run the `hello-world` image from the Docker public registry.

		$ docker run hello-world
	
	The `run` command automatically pulls a `hello-world` image from Docker's
	official images.

3. Start a registry on your localhost.

		$ docker run -p 5000:5000 registry:2.0
		
	This starts a registry on your `DOCKER_HOST` running on port `5000`. 
	
3. List your images.
	
		 $ docker images
		 REPOSITORY     TAG     IMAGE ID      CREATED       VIRTUAL SIZE
		 registry       2.0     bbf0b6ffe923  3 days ago    545.1 MB
		 golang         1.4     121a93c90463  5 days ago    514.9 MB
		 hello-world    latest  e45a5af57b00  3 months ago  910 B
  
    Your list should include a `hello-world` image from the earlier run.

4. Retag the `hello-world` image for your local repoistory.

		$ docker tag hello-world:latest localhost:5000/hello-mine:latest

	 The command labels a `hello-world:latest` using a new tag in the
	 `[REGISTRYHOST/]NAME[:TAG]` format.  The `REGISTRYHOST` is this case is
	 `localhost`. In a Mac OSX environment, you'd substitute `$(boot2docker
	 ip):5000` for the `localhost`.
	
5. List your new image.

		 $ docker images
		 REPOSITORY                  TAG          IMAGE ID      CREATED       VIRTUAL SIZE
		 registry                    2.0     bbf0b6ffe923  3 days ago    545.1 MB
		 golang                      1.4     121a93c90463  5 days ago    514.9 MB
		 hello-world                 latest  e45a5af57b00  3 months ago  910 B		 
		 localhost:5000/hello-mine   latest  ef5a5gf57b01  3 months ago  910 B
	
	 You should see your new image in your listing.

6. Push this new image to your local registry.

		$ docker push localhost:5000/hello-mine:latest
		The push refers to a repository [localhost:5000/hello-mine] (len: 1)
		e45a5af57b00: Image already exists 
		31cbccb51277: Image successfully pushed 
		511136ea3c5a: Image already exists 
		Digest: sha256:a1b13bc01783882434593119198938b9b9ef2bd32a0a246f16ac99b01383ef7a
		
7. Use the `curl` command and the Docker Registry API v2 to list your
   image in the registry:
   
		$ curl -v -X GET http://localhost:5000/v2/hello-mine/tags/list
		* Hostname was NOT found in DNS cache
		*   Trying 127.0.0.1...
		* Connected to localhost (127.0.0.1) port 5000 (#0)
		> GET /v2/hello-mine/tags/list HTTP/1.1
		> User-Agent: curl/7.35.0
		> Host: localhost:5000
		> Accept: */*
		> 
		< HTTP/1.1 200 OK
		< Content-Type: application/json; charset=utf-8
		< Docker-Distribution-Api-Version: registry/2.0
		< Date: Sun, 12 Apr 2015 01:29:47 GMT
		< Content-Length: 40
		< 
		{"name":"hello-mine","tags":["latest"]}
		* Connection #0 to host localhost left intact
		
	You can also get this information by entering the
	`http://localhost:5000/v2/hello-mine/tags/list` address in your browser.
			
8. Remove all the unused images from your local environment:

		$ docker rmi -f $(docker images -q -a )

	This command is for illustrative purposes; removing the image forces any `run`
	to pull from a registry rather than a local cache. If you run `docker images`
	after this you should not see any instance of `hello-world` or `hello-mine` in
	your images list.
	
		 $ docker images
		 REPOSITORY      TAG      IMAGE ID      CREATED       VIRTUAL SIZE
		 registry         2.0     bbf0b6ffe923  3 days ago    545.1 MB
		 golang           1.4     121a93c90463  5 days ago    514.9 MB
	
9. Try running `hello-mine`.

		$ docker run hello-mine
		Unable to find image 'hello-mine:latest' locally
		Pulling repository hello-mine
		FATA[0001] Error: image library/hello-mine:latest not found 
		
	The `run` command fails because your new image doesn't exist in the Docker public
	registry.

10. Now, try running the image but specifying the image's registry:

		$ docker run localhost:5000/hello-mine

	If you run `docker images` after this you'll fine a `hello-mine` instance. 
		
### Making Docker's official registry image production ready

Docker's official image is for simple tests or debugging. Its configuration is
unsuitable for most production instances. For example, any client with access to
the server's IP can push and pull images to it. See the next section for
information on making this image production ready.

## Understand production deployment

When deploying a registry for a production deployment you should consider these
factors:

<table>
  <tr>
  	<th align="left">
  		backend storage
  	</th>
  	<td>
  		Where should you store the images? 
  	</td>
  </tr>
  <tr>
  	<th align="left">
  		access and/or authentication
  	</th>
  	<td>
  		Should users have full or controlled access? This can depend on whether
  		you are serving images to the public or internally to your company only.
  	</td>
  </tr>
   <tr>
  	<th align="left">
  		debugging
  	</th>
  	<td>
  		When problems or issues arise, do you have the means of solving them. Logs
  		are useful as is reporting to see trends.
  	</td>
  </tr>
  <tr>
  	<th align="left">
  		caching
  	</th>
  	<td>
  		Quickly retrieving images can be crucial if you are relying on images for
  		tests, builds, or other automated systems.
  	</td>
  </tr>     
</table>

You can configure your registry features to adjust for these factors. You do
this by specifying options on the command line or, more typically, by writing a
registry configuration file. The configuration file is in YAML format.

Docker's official repository image is preconfigured using the following
configuration file:

```yaml
version: 0.1
log:
  level: debug
  fields:
    service: registry
    environment: development
storage:
  cache:
      layerinfo: inmemory
  filesystem:
      rootdirectory: /tmp/registry-dev
  maintenance:
		uploadpurging:
			enabled: false
http:
  addr: :5000
  secret: asecretforlocaldevelopment
  debug:
      addr: localhost:5001
redis:
  addr: localhost:6379
  pool:
    maxidle: 16
    maxactive: 64
    idletimeout: 300s
  dialtimeout: 10ms
  readtimeout: 10ms
  writetimeout: 10ms
notifications:
  endpoints:
      - name: local-8082
        url: http://localhost:5003/callback
        headers:
           Authorization: [Bearer <an example token>]
        timeout: 1s
        threshold: 10
        backoff: 1s
        disabled: true
      - name: local-8083
        url: http://localhost:8083/callback
        timeout: 1s
        threshold: 10
        backoff: 1s
        disabled: true
```

This configuration is very basic and you can see it would present some problems
in a production. For example, the `http` section details the configuration for
the HTTP server that hosts the registry. The server is not using even the most
minimal transport layer security (TLS). Let's configure that in the next section. 

## Configure TLS on a registry server

In this section, you configure TLS on the server to enable communication through
the `https` protocol. Enabling TLS on the server is the minimum layer of
security recommended for running a registry behind a corporate firewall. One way
to do this is to build your own registry image.  

### Download the source and generate certificates

1. [Download the registry
source](https://github.com/docker/distribution/releases/tag/v2.0.0).

	Alternatively, use the `git clone` command if you are more comfortable with that.

2. Unpack the the downloaded package into a local directory.

	The package creates a `distribution` directory.

3. Change to the root of the new `distribution` directory.

		$ cd distribution

4. Make a `certs` subdirectory.
	
		$ mkdir certs
		
5. Use SSL to generate some self-signed certificates.
	
		$ openssl req \
				 -newkey rsa:2048 -nodes -keyout certs/domain.key \
				 -x509 -days 365 -out certs/domain.crt
				 
	This command prompts you for basic information it needs to create the certificates.
				 
6. List the contents of the `certs` directory.

		$ ls certs
		domain.crt  domain.key

	When you build this container, the `certs` directory and its contents
	automatically get copied also.
	
### Add TLS to the configuration

The `distribution` repo includes sample registry configurations in the `cmd`
subdirectory. In this section, you edit one of these configurations to add TLS
support. 
		
1. Edit the `./cmd/registry/config.yml`  file.

		$ vi ./cmd/registry/config.yml 

2. Locate the `http` block.

		http:
				addr: :5000
				secret: asecretforlocaldevelopment
				debug:
						addr: localhost:5001

3. Add a `tls` block for the server's self-signed certificates:

		http:
				addr: :5000
				secret: asecretforlocaldevelopment
				debug:
						addr: localhost:5001
				tls:
					certificate: /go/src/github.com/docker/distribution/certs/domain.crt
					key: /go/src/github.com/docker/distribution/certs/domain.key	
		
	You provide the paths to the certificates in the container. If you want
	two-way authentication across the layer, you can add an optional `clientcas`
	section.
	
4. Save and close the file.

		
### Build and run your registry image

1. Build your registry image.

		$ docker build -t secure_registry .
	
2. Run your new image.

		$ docker run -p 5000:5000 secure_registry:latest
		time="2015-04-12T03:06:18.616502588Z" level=info msg="endpoint local-8082 disabled, skipping" environment=development instance.id=bf33c9dc-2564-406b-97c3-6ee69dc20ec6 service=registry 
		time="2015-04-12T03:06:18.617012948Z" level=info msg="endpoint local-8083 disabled, skipping" environment=development instance.id=bf33c9dc-2564-406b-97c3-6ee69dc20ec6 service=registry 
		time="2015-04-12T03:06:18.617190113Z" level=info msg="using inmemory layerinfo cache" environment=development instance.id=bf33c9dc-2564-406b-97c3-6ee69dc20ec6 service=registry 
		time="2015-04-12T03:06:18.617349067Z" level=info msg="listening on :5000, tls" environment=development instance.id=bf33c9dc-2564-406b-97c3-6ee69dc20ec6 service=registry 
		time="2015-04-12T03:06:18.628589577Z" level=info msg="debug server listening localhost:5001" 
		2015/04/12 03:06:28 http: TLS handshake error from 172.17.42.1:44261: remote error: unknown certificate authority
		
		Watch the messages at startup. You should see that `tls` is running.
		
3. Use `curl` to verify that you can connect over `https`.

		$ curl -v https://localhost:5000
		* Rebuilt URL to: https://localhost:5000/
		* Hostname was NOT found in DNS cache
		*   Trying 127.0.0.1...
		* Connected to localhost (127.0.0.1) port 5000 (#0)
		* successfully set certificate verify locations:
		*   CAfile: none
			CApath: /etc/ssl/certs
		* SSLv3, TLS handshake, Client hello (1):
		* SSLv3, TLS handshake, Server hello (2):
		* SSLv3, TLS handshake, CERT (11):
		* SSLv3, TLS alert, Server hello (2):
		* SSL certificate problem: self signed certificate
		* Closing connection 0
		curl: (60) SSL certificate problem: self signed certificate
		More details here: http://curl.haxx.se/docs/sslcerts.html
	
## Configure Nginx with a v1 and v2 registry

This sections describes how to  user `docker-compose` to run a combined version
1 and version 2.0 registry behind an `nginx` proxy. The combined registry is
accessed at `localhost:5000`. If a `docker` client has a version less than 1.6,
Nginx will route its requests to the 1.0 registry. Requests from newer clients
will route to the 2.0 registry.

This procedure uses the same `distribution` directory you created in the last
procedure. The directory includes an example `compose` configuration. 

### Install Docker Compose

1. Open a new terminal on the host with your `distribution` directory.

2. Get the `docker-compose` binary.

		$ sudo wget https://github.com/docker/compose/releases/download/1.1.0/docker-compose-`uname  -s`-`uname -m` -O /usr/local/bin/docker-compose

	This command installs the binary in the `/usr/local/bin` directory. 
	
3. Add executable permissions to the binary.

		$  sudo chmod +x /usr/local/bin/docker-compose
		

### Do some housekeeping

1. Remove any previous images.

		$ docker rmi -f $(docker images -q -a )
		
	 This step is a house keeping step. It prevents you from mistakenly picking up
	 an old image as you work through this example.
		
2. Edit the `distribution/cmd/registry/config.yml` file and remove the `tls` block.

	If you worked through the previous example, you'll have a `tls` block. 

4. Save any changes and close the file.

### Configure SSL

1. Change to the `distribution/contrib/compose/nginx` directory.

	This directory contains configuration files for Nginx and both registries.
	
2. Use SSL to generate some self-signed certificates.
	
		$ openssl req \
				 -newkey rsa:2048 -nodes -keyout domain.key \
				 -x509 -days 365 -out domain.crt
				 
	 This command prompts you for basic information it needs to create certificates.
				 
3. Edit the `Dockerfile`and add the following lines.

		COPY domain.crt /etc/nginx/domain.crt
		COPY domain.key /etc/nginx/domain.key
		
	When you are done, the file looks like the following.
	
		FROM nginx:1.7

		COPY nginx.conf /etc/nginx/nginx.conf
		COPY registry.conf /etc/nginx/conf.d/registry.conf
		COPY docker-registry.conf /etc/nginx/docker-registry.conf
		COPY docker-registry-v2.conf /etc/nginx/docker-registry-v2.conf
		COPY domain.crt /etc/nginx/domain.crt
		COPY domain.key /etc/nginx/domain.key

4. Save and close the `Dockerfile` file.
		
5. Edit the `registry.conf` file and add the following configuration. 

		 ssl on;
			ssl_certificate /etc/nginx/domain.crt;
			ssl_certificate_key /etc/nginx/domain.key;
			
	This is an `nginx` configuration file.

6. Save and close the `registry.conf` file.

### Build and run

1. Go up to the `distribution/contrib/compose` directory

	This directory includes a single `docker-compose.yml` configuration.
	
		nginx:
			build: "nginx"
			ports:
				- "5000:5000"
			links:
				- registryv1:registryv1
				- registryv2:registryv2
		registryv1:
			image: registry
			ports:
				- "5000"
		registryv2:
			build: "../../"
			ports:
				- "5000"

 This configuration builds a new `nginx` image as specified by the
 `nginx/Dockerfile` file. The 1.0 registry comes from Docker's official public
 image. Finally, the registry 2.0 image is built from the
 `distribution/Dockerfile` you've used previously.

2. Get a registry 1.0 image.

		$ docker pull registry:0.9.1 

	The Compose configuration looks for this image locally. If you don't do this
	step, later steps can fail.
	
3. Build `nginx`, the registry 2.0 image, and 

		$ docker-compose build
		registryv1 uses an image, skipping
		Building registryv2...
		Step 0 : FROM golang:1.4
		
		...
		
		Removing intermediate container 9f5f5068c3f3
		Step 4 : COPY docker-registry-v2.conf /etc/nginx/docker-registry-v2.conf
		 ---> 74acc70fa106
		Removing intermediate container edb84c2b40cb
		Successfully built 74acc70fa106
		
	The commmand outputs its progress until it completes.

4. Start your configuration with compose.

		$ docker-compose up
		Recreating compose_registryv1_1...
		Recreating compose_registryv2_1...
		Recreating compose_nginx_1...
		Attaching to compose_registryv1_1, compose_registryv2_1, compose_nginx_1
		...
	

5. In another terminal, display the running configuration.

		$ docker ps
		CONTAINER ID        IMAGE                       COMMAND                CREATED             STATUS              PORTS                                     NAMES
		a81ad2557702        compose_nginx:latest        "nginx -g 'daemon of   8 minutes ago       Up 8 minutes        80/tcp, 443/tcp, 0.0.0.0:5000->5000/tcp   compose_nginx_1        
		0618437450dd        compose_registryv2:latest   "registry cmd/regist   8 minutes ago       Up 8 minutes        0.0.0.0:32777->5000/tcp                   compose_registryv2_1   
		aa82b1ed8e61        registry:latest             "docker-registry"      8 minutes ago       Up 8 minutes        0.0.0.0:32776->5000/tcp                   compose_registryv1_1   
	
### Explore a bit

1. Check for TLS on your `nginx` server.

		$ curl -v https://localhost:5000
		* Rebuilt URL to: https://localhost:5000/
		* Hostname was NOT found in DNS cache
		*   Trying 127.0.0.1...
		* Connected to localhost (127.0.0.1) port 5000 (#0)
		* successfully set certificate verify locations:
		*   CAfile: none
			CApath: /etc/ssl/certs
		* SSLv3, TLS handshake, Client hello (1):
		* SSLv3, TLS handshake, Server hello (2):
		* SSLv3, TLS handshake, CERT (11):
		* SSLv3, TLS alert, Server hello (2):
		* SSL certificate problem: self signed certificate
		* Closing connection 0
		curl: (60) SSL certificate problem: self signed certificate
		More details here: http://curl.haxx.se/docs/sslcerts.html
		
2. Tag the `v1` registry image.

		 $ docker tag registry:latest localhost:5000/registry_one:latest

2. Push it to the localhost.

		 $ docker push localhost:5000/registry_one:latest
		
	If you are using the 1.6 Docker client, this pushes the image the `v2 `registry.

4. Use `curl` to list the image in the registry.

			$ curl -v -X GET http://localhost:32777/v2/registry_one/tags/list
			* Hostname was NOT found in DNS cache
			*   Trying 127.0.0.1...
			* Connected to localhost (127.0.0.1) port 32777 (#0)
			> GET /v2/registry_one/tags/list HTTP/1.1
			> User-Agent: curl/7.36.0
			> Host: localhost:32777
			> Accept: */*
			> 
			< HTTP/1.1 200 OK
			< Content-Type: application/json; charset=utf-8
			< Docker-Distribution-Api-Version: registry/2.0
			< Date: Tue, 14 Apr 2015 22:34:13 GMT
			< Content-Length: 39
			< 
			{"name":"registry1","tags":["latest"]}
			* Connection #0 to host localhost left intact
		
	This example refers to the specific port assigned to the 2.0 registry. You saw
	this port earlier, when you used `docker ps` to show your running containers.

