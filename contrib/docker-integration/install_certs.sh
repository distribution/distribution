#!/bin/sh

hostname=$1
if [ "$hostname" == "" ]; then
	hostname="localhost"
fi

mkdir -p /etc/docker/certs.d/$hostname:5440
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/$hostname:5440/ca.crt

mkdir -p /etc/docker/certs.d/$hostname:5441
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/$hostname:5441/ca.crt

mkdir -p /etc/docker/certs.d/$hostname:5442
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/$hostname:5442/ca.crt
cp ./nginx/ssl/registry-ca+client-client.pem /etc/docker/certs.d/$hostname:5442/client.cert
cp ./nginx/ssl/registry-ca+client-client-key.pem /etc/docker/certs.d/$hostname:5442/client.key

mkdir -p /etc/docker/certs.d/$hostname:5443
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/$hostname:5443/ca.crt
cp ./nginx/ssl/registry-ca+client+bad-client.pem /etc/docker/certs.d/$hostname:5443/client.cert
cp ./nginx/ssl/registry-ca+client+bad-client-key.pem /etc/docker/certs.d/$hostname:5443/client.key

mkdir -p /etc/docker/certs.d/$hostname:5444
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/$hostname:5444/ca.crt
cp ./nginx/ssl/registry-ca+client-client.pem /etc/docker/certs.d/$hostname:5444/client.cert
cp ./nginx/ssl/registry-ca+client-client-key.pem /etc/docker/certs.d/$hostname:5444/client.key

mkdir -p /etc/docker/certs.d/$hostname:5447
cp ./nginx/ssl/registry-ca+client-client.pem /etc/docker/certs.d/$hostname:5447/client.cert
cp ./nginx/ssl/registry-ca+client-client-key.pem /etc/docker/certs.d/$hostname:5447/client.key

mkdir -p /etc/docker/certs.d/localhost:5448
cp ./nginx/ssl/registry-ca+client-ca.pem /etc/docker/certs.d/localhost:5448/ca.crt
