FROM nginx

COPY certs /etc/ssl/certs
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY docker-registry.conf /etc/nginx/docker-registry.conf
COPY htpasswd /etc/nginx/htpasswd

EXPOSE 443

