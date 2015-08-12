<!--[metadata]>
+++
title = "Authenticating proxy with nginx"
description = "Restricting access to your registry using a proxy"
keywords = ["registry, service, images, repository, authentication"]
+++
<![end-metadata]-->

# Authenticating proxy with nginx

With this method, you implement basic authentication in a reverse proxy that sits in front of your registry.

While this model gives you the ability to use whatever authentication backend you want through a secondary authentication mechanism implemented inside your proxy, it also requires that you move TLS termination from the Registry to the proxy itself.

Furthermore, introducing an extra http layer in your communication pipeline will make it more complex to deploy, maintain, and debug, and will possibly create issues (typically, nginx does buffer client requests to disk, opening the door to a host of problems if you are dealing with huge images and a lot of traffic).

### Requirements

You should have followed entirely the basic [deployment guide](deploying.md).

If you have not, please take the time to do so.

At this point, it's assumed that:

 * you understand Docker security requirements, and how to configure your docker engines properly
 * you have installed Docker Compose
 * it's HIGHLY recommended that you get a certificate from a known CA instead of self-signed certificates
 * inside the current directory, you have a X509 `domain.crt` and `domain.key`, for the CN `myregistrydomain.com` (or whatever domain name you want to use)
 * be sure you have stopped and removed any previously running registry (typically `docker stop registry && docker rm -v registry`)

### Setting things up

Read again the requirements.

Ready?

Run the following:

```
mkdir -p auth
mkdir -p data

# This is the main nginx configuration you will use
cat <<EOF > auth/registry.conf
upstream docker-registry {
  server registry:5000;
}

server {
  listen 443 ssl;
  server_name myregistrydomain.com;

  # SSL
  ssl_certificate /etc/nginx/conf.d/domain.crt;
  ssl_certificate_key /etc/nginx/conf.d/domain.key;

  # disable any limits to avoid HTTP 413 for large image uploads
  client_max_body_size 0;

  # required to avoid HTTP 411: see Issue #1486 (https://github.com/docker/docker/issues/1486)
  chunked_transfer_encoding on;

  location /v2/ {
    # Do not allow connections from docker 1.5 and earlier
    # docker pre-1.6.0 did not properly set the user agent on ping, catch "Go *" user agents
    if (\$http_user_agent ~ "^(docker\/1\.(3|4|5(?!\.[0-9]-dev))|Go ).*\$" ) {
      return 404;
    }

    # To add basic authentication to v2 use auth_basic setting plus add_header
    auth_basic "Registry realm";
    auth_basic_user_file /etc/nginx/conf.d/htpasswd;
    add_header 'Docker-Distribution-Api-Version' 'registry/2.0' always;

    proxy_pass                          http://docker-registry;
    proxy_set_header  Host              \$http_host;   # required for docker client's sake
    proxy_set_header  X-Real-IP         \$remote_addr; # pass on real client's IP
    proxy_set_header  X-Forwarded-For   \$proxy_add_x_forwarded_for;
    proxy_set_header  X-Forwarded-Proto \$scheme;
    proxy_read_timeout                  900;
  }
}
EOF

# Now, create a password file for "testuser" and "testpassword"
htpasswd -bn testuser testpassword > auth/htpasswd

# Copy over your certificate files
cp domain.crt auth
cp domain.key auth

# Now create your compose file

cat <<EOF > docker-compose.yml
nginx:
  image: "nginx:1.9"
  ports:
    - 5043:443
  links:
    - registry:registry
  volumes:
    - `pwd`/auth/:/etc/nginx/conf.d

registry:
  image: registry:2
  ports:
    - 127.0.0.1:5000:5000
  environment:
    REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY: /data
  volumes:
    - `pwd`/data:/data
EOF
```

### Starting and stopping

That's it. You can now:

 * `docker-compose up -d` to start your registry
 * `docker login myregistrydomain.com:5043` (using `testuser` and `testpassword`)
 * `docker tag ubuntu myregistrydomain.com:5043/toto`
 * `docker push myregistrydomain.com:5043/toto`

### Docker still complains about the certificate?

That's certainly because you are using a self-signed certificate, despite the warnings.

If you really insist on using these, you have to trust it at the OS level.

Usually, on Ubuntu this is done with:

    cp auth/domain.crt /usr/local/share/ca-certificates/myregistrydomain.com.crt
    update-ca-certificates

... and on RedHat with:

    cp auth/domain.crt /etc/pki/ca-trust/source/anchors/myregistrydomain.com.crt
    update-ca-trust

Now:

 * `service docker stop && service docker start` (or any other way you use to restart docker)
 * `docker-compose up -d` to bring your registry up
