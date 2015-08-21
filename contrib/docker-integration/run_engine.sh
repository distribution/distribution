#!/bin/sh
set -e
set -x

DOCKER_GRAPHDRIVER=${DOCKER_GRAPHDRIVER:-overlay}
EXEC_DRIVER=${EXEC_DRIVER:-native}

# Set IP address in /etc/hosts for localregistry
IP=$(ifconfig eth0|grep "inet addr:"| cut -d: -f2 | awk '{ print $1}')
echo "$IP localregistry" >> /etc/hosts

sh install_certs.sh localregistry

docker --daemon --log-level=panic \
	--storage-driver="$DOCKER_GRAPHDRIVER" --exec-driver="$EXEC_DRIVER"
