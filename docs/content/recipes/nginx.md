---
description: Restricting access to your registry using a nginx proxy
keywords: registry, on-prem, images, tags, repository, distribution, nginx, proxy, authentication, TLS, recipe, advanced
title: Authenticate proxy with nginx
---

## Use-case

People already relying on a nginx proxy to authenticate their users to other
services might want to leverage it and have Registry communications tunneled
through the same pipeline.

Usually, that includes enterprise setups using LDAP/AD on the backend and a SSO
mechanism fronting their internal http portal.

### Alternatives

If you just want authentication for your registry, and are happy maintaining
users access separately, you should really consider sticking with the native
[basic auth registry feature](../about/deploying.md#native-basic-auth).

### Solution

With the method presented here, you implement basic authentication for docker
engines in a reverse proxy that sits in front of your registry.

While we use a simple htpasswd file as an example, any other nginx
authentication backend should be fairly easy to implement once you are done with
the example.

We also implement push restriction (to a limited user group) for the sake of the
example. Again, you should modify this to fit your mileage.

### Gotchas

While this model gives you the ability to use whatever authentication backend
you want through the secondary authentication mechanism implemented inside your
proxy, it also requires that you move TLS termination from the Registry to the
proxy itself.

> **Note**: It is not recommended to bind your registry to `localhost:5000` without
> authentication. This creates a potential loophole in your registry security.
> As a result, anyone who can log on to the server where your registry is running
> can push images without authentication.

Furthermore, introducing an extra http layer in your communication pipeline
makes it more complex to deploy, maintain, and debug. Make sure the extra
complexity is required.

For instance, Amazon's Elastic Load Balancer (ELB) in HTTPS mode already sets
the following client header:

```none
X-Real-IP
X-Forwarded-For
X-Forwarded-Proto
```

So if you have an Nginx instance sitting behind it, remove these lines from the
example config below:

```none
proxy_set_header  Host              $http_host;   # required for docker client's sake
proxy_set_header  X-Real-IP         $remote_addr; # pass on real client's IP
proxy_set_header  X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header  X-Forwarded-Proto $scheme;
```

Otherwise Nginx resets the ELB's values, and the requests are not routed
properly. For more information, see
[#970](https://github.com/distribution/distribution/issues/970).

## Setting things up

Review the [requirements](../#requirements), then follow these steps.

1. Create the required directories

   ```console
   $ mkdir -p auth data
   ```

2. Create the main nginx configuration. Paste this code block into a new file called `auth/nginx.conf`:

   ```conf
   events {
       worker_connections  1024;
   }

   http {

     upstream docker-registry {
       server registry:5000;
     }

     ## Set a variable to help us decide if we need to add the
     ## 'Docker-Distribution-Api-Version' header.
     ## The registry always sets this header.
     ## In the case of nginx performing auth, the header is unset
     ## since nginx is auth-ing before proxying.
     map $upstream_http_docker_distribution_api_version $docker_distribution_api_version {
       '' 'registry/2.0';
     }

     server {
       listen 443 ssl;
       server_name myregistrydomain.com;

       # SSL
       ssl_certificate /etc/nginx/conf.d/domain.crt;
       ssl_certificate_key /etc/nginx/conf.d/domain.key;

       # Recommendations from https://raymii.org/s/tutorials/Strong_SSL_Security_On_nginx.html
       ssl_protocols TLSv1.1 TLSv1.2;
       ssl_ciphers 'EECDH+AESGCM:EDH+AESGCM:AES256+EECDH:AES256+EDH';
       ssl_prefer_server_ciphers on;
       ssl_session_cache shared:SSL:10m;

       # disable any limits to avoid HTTP 413 for large image uploads
       client_max_body_size 0;

       # required to avoid HTTP 411: see Issue #1486 (https://github.com/moby/moby/issues/1486)
       chunked_transfer_encoding on;

       location /v2/ {
         # Do not allow connections from docker 1.5 and earlier
         # docker pre-1.6.0 did not properly set the user agent on ping, catch "Go *" user agents
         if ($http_user_agent ~ "^(docker\/1\.(3|4|5(?!\.[0-9]-dev))|Go ).*$" ) {
           return 404;
         }

         # To add basic authentication to v2 use auth_basic setting.
         auth_basic "Registry realm";
         auth_basic_user_file /etc/nginx/conf.d/nginx.htpasswd;

         ## If $docker_distribution_api_version is empty, the header is not added.
         ## See the map directive above where this variable is defined.
         add_header 'Docker-Distribution-Api-Version' $docker_distribution_api_version always;

         proxy_pass                          http://docker-registry;
         proxy_set_header  Host              $http_host;   # required for docker client's sake
         proxy_set_header  X-Real-IP         $remote_addr; # pass on real client's IP
         proxy_set_header  X-Forwarded-For   $proxy_add_x_forwarded_for;
         proxy_set_header  X-Forwarded-Proto $scheme;
         proxy_read_timeout                  900;
       }
     }
   }
   ```

3. Create a password file `auth/nginx.htpasswd` for "testuser" and "testpassword".

   ```console
   $ docker run --rm --entrypoint htpasswd httpd -Bbn testuser testpassword > auth/nginx.htpasswd
   ```

   > **Note**: If you do not want to use `bcrypt`, you can omit the `-B` parameter.

4. Copy your certificate files to the `auth/` directory.

   ```console
   $ cp domain.crt auth
   $ cp domain.key auth
   ```

5. Create the compose file. Paste the following YAML into a new file called `docker-compose.yml`.

   ```yaml
   services:
       nginx:
         # Note : Only nginx:alpine supports bcrypt.
         # If you don't need to use bcrypt, you can use a different tag.
         # Ref. https://github.com/nginxinc/docker-nginx/issues/29
         image: "nginx:alpine"
         ports:
           - 5043:443
         depends_on:
           - registry
         volumes:
           - ./auth:/etc/nginx/conf.d
           - ./auth/nginx.conf:/etc/nginx/nginx.conf:ro

       registry:
         image: registry:2
         volumes:
           - ./data:/var/lib/registry
   ```

## Starting and stopping

Now, start your stack:

```consonle
$ docker compose up -d
```

Login with a "push" authorized user (using `testuser` and `testpassword`), then
tag and push your first image:

```console
$ docker login -u=testuser -p=testpassword -e=root@example.ch myregistrydomain.com:5043
$ docker tag ubuntu myregistrydomain.com:5043/test
$ docker push myregistrydomain.com:5043/test
$ docker pull myregistrydomain.com:5043/test
```
