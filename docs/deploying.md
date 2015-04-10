page_title: Deploying a registry service
page_description: Explains how to deploy a registry service
page_keywords: registry, service, images, repository

# Deploying a registry service

This section explains how to deploy a Docker Registry Service either privately
for your own company or publicly for other users. For example, your company may
require a private registry to support your continuous integration (CI) system as
it builds new releases or test servers. Alternatively, your company may have a
large number of products or services with images you wish to server in a branded
manner.

Docker's public registry maintains a default `registry` image to assist you in the
deployment process. This registry image is sufficient for running local tests
but is insufficient for production. For production you should configure and
build your own custom registry image from the `docker/distribution` code.


## Simple example with the official image

In this section, you create a local registry using Docker's official image. You
push an image to, and then pull the same image from, the registry. This a good
exercise for understanding the basic interactions a client has with a
local registry.

1. Install Docker.

2. Run the `hello-world` image from the Docker public registry.

		$ docker run hello-world
	
	The `run` command automatically pulls the image from Docker's official images.

3. Start a registry service on your localhost.

		$ docker run -p 5000:5000 registry
		
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

5. Push this new image to your local registry.

		$ docker push localhost:5000/hello-mine:latest
			
6. Remove all the unused images from your local environment:

		$ docker rmi -f $(docker images -q -a )

	This command is for illustrative purposes; removing the image forces any `run`
	to pull from a registry rather than a local cache. If you run `docker images`
	after this you should not see any instance of `hello-world` or `hello-mine` in
	your images list.
	
		 $ docker images
		 REPOSITORY      TAG      IMAGE ID      CREATED       VIRTUAL SIZE
		 registry         2.0     bbf0b6ffe923  3 days ago    545.1 MB
		 golang           1.4     121a93c90463  5 days ago    514.9 MB
	
7. Try running `hello-mine`.

		$ docker run hello-mine
		Unable to find image 'hello-mine:latest' locally
		Pulling repository hello-mine
		FATA[0001] Error: image library/hello-mine:latest not found 
		
	The `run` command fails because your new image doesn't exist in the Docker public
	registry.

8. Now, try running the image but specifying the image's registry:

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
  		Do users should have full or controlled access? This can depend on whether
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

Docker's official repository image it is preconfigured using the following
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
security recommended for running a registry behind a corporate firewall. The
easiest way to do this is to build your own registry image.  

### Download the registry source and generated certificates

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
				 
				 
### Add the certificates to the image

In this section, you copy the certifications from your `certs` directory into
your base image.
							 
1. Edit the `Dockerfile` and add a `CERTS_PATH` environment variable.
 
		ENV CERTS_PATH  /etc/docker/registry/certs

2. Add a line to make the `CERTS_PATH` in the filesystem.

		RUN mkdir -v $CERTS_PATH
		 
3. Add `RUN` instructions to hard link your new certifications into this path:

		RUN cp -lv ./certs/domain.crt $CERTS_PATH
		RUN cp -lv ./certs/domain.key $CERTS_PATH

		This copies your certifications into your container.
 			
4. Save your work.

	 At this point your Dockerfile should look like the following:
	 
		FROM golang:1.4

		ENV CONFIG_PATH /etc/docker/registry/config.yml
		ENV CERTS_PATH	/etc/docker/registry/certs
		ENV DISTRIBUTION_DIR /go/src/github.com/docker/distribution
		ENV GOPATH $DISTRIBUTION_DIR/Godeps/_workspace:$GOPATH

		WORKDIR $DISTRIBUTION_DIR
		COPY . $DISTRIBUTION_DIR
		RUN make PREFIX=/go clean binaries
		RUN mkdir -pv "$(dirname $CONFIG_PATH)"
		RUN mkdir -v $CERTS_PATH
		RUN cp -lv ./certs/domain.crt $CERTS_PATH
		RUN cp -lv ./certs/domain.key $CERTS_PATH
		RUN cp -lv ./cmd/registry/config.yml $CONFIG_PATH

5. Before you close the Dockerfile look for an instruction to copy the `config.yml` file.
	
		RUN cp -lv ./cmd/registry/config.yml $CONFIG_PATH
		
	This is the default registry configuration file. You'll need to edit the file
	to add TLS.
	
### Add TLS to the registry configuration
		
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
					certificate: /etc/docker/registry/certs/domain.crt
					key: /etc/docker/registry/certs/domain.key	
		
	You provide the paths to the certificates in the container. If you want
	two-way authentication across the layer, you can add an optional `clientcas`
	section.
	
4. Save and close the file.

		
### Run your new image

1. Build your registry image.

		$ docker build -t secure_registry .
	
2. Run your new image.

		$ docker run -p 5000:5000 secure_registry
		
		Watch the messages at startup. You should see that `tls` is running:

		ubuntu@ip-172-31-34-181:~/repos/distribution$ docker run -p 5000:5000 secure_registry
		time="2015-04-05T23:56:47Z" level=info msg="endpoint local-8082 disabled, skipping" app.id=3dd802ad-3bd4-4413-b56d-90c4acff41c7 environment=development service=registry 
		time="2015-04-05T23:56:47Z" level=info msg="endpoint local-8083 disabled, skipping" app.id=3dd802ad-3bd4-4413-b56d-90c4acff41c7 environment=development service=registry 
		time="2015-04-05T23:56:47Z" level=info msg="using inmemory layerinfo cache" app.id=3dd802ad-3bd4-4413-b56d-90c4acff41c7 environment=development service=registry 
		time="2015-04-05T23:56:47Z" level=info msg="listening on :5000, tls" app.id=3dd802ad-3bd4-4413-b56d-90c4acff41c7 environment=development service=registry 
		time="2015-04-05T23:56:47Z" level=info msg="debug server listening localhost:5001" 
		2015/04/05 23:57:23 http: TLS handshake error from 172.17.42.1:52057: remote error: unknown certificate authority
		
3. Use `curl` to verify that you can connect over `https`.

		$ curl https://localhost:5000
	
		
## Adding a middleware configuration

This section describes how to configure storage middleware in a registry.
Middleware allows the registry to server layers via a content delivery network
(CDN). This is useful for reducing requests to the storage layer.  

Currently, the registry supports [Amazon
Cloudfront](http://aws.amazon.com/cloudfront/).  You can only use Cloudfront in
conjunction with the S3 storage driver.

<table>
  <tr>
    <th>Parameter</th>
    <th>Description</th>
  </tr>
  <tr>
    <td><code>name</code></td>
    <td>The storage middleware name. Currently <code>cloudfront</code> is an accepted value.</td>
  </tr>
  <tr>
    <td><code>disabled<code></td>
    <td>Set to <code>false</code> to easily disable the middleware.</td>
  </tr>
  <tr>
    <td><code>options:</code></td>
    <td> 
    A set of key/value options to configure the middleware.
    <ul>
    <li><code>baseurl:</code> The Cloudfront base URL.</li>
    <li><code>privatekey:</code> The location of your AWS private key on the filesystem. </li>
    <li><code>keypairid:</code> The ID of your Cloudfront keypair. </li>
 		<li><code>duration:</code> The duration in minutes for which the URL is valid. Default is 20. </li>
 		</ul>
    </td>
  </tr>
</table>

The following example illustrates these values:

```
middleware:
    storage:
        - name: cloudfront
          disabled: false
          options:
             baseurl: http://d111111abcdef8.cloudfront.net
             privatekey: /path/to/asecret.pem
             keypairid: asecret
             duration: 60
```


>**Note**: Cloudfront keys exist separately to other AWS keys.  See
>[the documentation on AWS credentials](http://docs.aws.amazon.com/AWSSecurityCredentials/1.0/
>AboutAWSCredentials.html#KeyPairs) for more information.


**TODO(stevvooe): Need a "best practice" configuration overview. Perhaps, we can point to a documentation section.


# Configure nginx to deploy alongside v1 registry

This sections describes how to configure nginx to proxy to both a v1 and v2
registry. Nginx will handle routing of to the correct registry based on the
URL and Docker client version.

## Example configuration
With v1 registry running at `localhost:5001` and v2 registry running at
`localhost:5002`.  Add this to `/etc/nginx/conf.d/registry.conf`.
```
server {
  listen 5000;
  server_name localhost;

  ssl on;
  ssl_certificate /etc/docker/registry/certs/domain.crt;
  ssl_certificate_key /etc/docker/registry/certs/domain.key;

  client_max_body_size 0; # disable any limits to avoid HTTP 413 for large image uploads

  # required to avoid HTTP 411: see Issue #1486 (https://github.com/docker/docker/issues/1486)
  chunked_transfer_encoding on;

  location /v2/ {
    # Do not allow connections from docker 1.5 and earlier
    # docker pre-1.6.0 did not properly set the user agent on ping, catch "Go *" user agents
    if ($http_user_agent ~ "^(docker\/1\.(3|4|5(?!\.[0-9]-dev))|Go ).*$" ) {
      return 404;
    }

    proxy_pass                       http://localhost:5002;
    proxy_set_header  Host           $http_host;   # required for docker client's sake
    proxy_set_header  X-Real-IP      $remote_addr; # pass on real client's IP
    proxy_read_timeout               900;
  }

  location / {
    proxy_pass                       http://localhost:5001;
    proxy_set_header  Host           $http_host;   # required for docker client's sake
    proxy_set_header  X-Real-IP      $remote_addr; # pass on real client's IP
    proxy_set_header  Authorization  ""; # see https://github.com/docker/docker-registry/issues/170
    proxy_read_timeout               900;
  }
}
```

## Running nginx without a v1 registry
When running a v2 registry behind nginx without a v1 registry, the `/v1/` endpoint should
be explicitly configured to return a 404 if only the `/v2/` route is proxied. This
is needed due to the v1 registry fallback logic within Docker 1.5 and 1.6 which will attempt
to retrieve content from the v1 endpoint if no content was retrieved from v2.

Add this location block to explicitly block v1 requests.
```
localhost /v1/ {
	return 404;
}
```
