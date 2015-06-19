# Docker registry with authentication / tls

1. create htaccess file in nginx/htpassd
2. update your nginx/nginx.conf
3. add your certs ( nginx/certs/ssl.key and ssl.crt )

### Build the nginx image
```
docker-compose build
```

### Start nginx and registry
```
docker-compose up

```

