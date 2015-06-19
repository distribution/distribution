#!/usr/bin/env bash
set -e

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

# Load the helpers.
#. helpers.bash

TESTS=${@:-.}

# Drivers to use for Docker engines the tests are going to create.
STORAGE_DRIVER=${STORAGE_DRIVER:-overlay}
EXEC_DRIVER=${EXEC_DRIVER:-native}


function execute() {
	>&2 echo "++ $@"
	eval "$@"
}

# Set IP address in /etc/hosts for localregistry
IP=$(ifconfig eth0|grep "inet addr:"| cut -d: -f2 | awk '{ print $1}')
execute echo "$IP localregistry" >> /etc/hosts

# Setup certificates
execute sh install_certs.sh localregistry

# Start the docker engine.
execute docker --daemon --log-level=panic \
	--storage-driver="$STORAGE_DRIVER" --exec-driver="$EXEC_DRIVER" &
DOCKER_PID=$!

# Wait for it to become reachable.
tries=10
until docker version &> /dev/null; do
	(( tries-- ))
	if [ $tries -le 0 ]; then
		echo >&2 "error: daemon failed to start"
		exit 1
	fi
	sleep 1
done

execute time docker-compose build

execute docker-compose up -d

# Run the tests.
execute time bats -p $TESTS

