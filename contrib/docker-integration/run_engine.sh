#!/bin/sh
set -e
set -x

# Set IP address in /etc/hosts for localregistry
IP=$(ifconfig eth0|grep "inet addr:"| cut -d: -f2 | awk '{ print $1}')
echo "$IP localregistry" >> /etc/hosts

sh install_certs.sh localregistry

docker --daemon -H "0.0.0.0:$ENGINE_PORT" --log-level=panic \
	--storage-driver="$DOCKER_GRAPHDRIVER" --exec-driver="$EXEC_DRIVER"
