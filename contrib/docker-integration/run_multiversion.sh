#!/usr/bin/env bash

# Run the integration tests with multiple versions of the Docker engine

set -e
set -x

# Don't use /tmp because this isn't available in boot2docker
tmpdir_template="`pwd`/docker-versions.XXXXX"
tmpdir=`mktemp -d "$tmpdir_template"`
trap "rm -rf $tmpdir" EXIT

if [ "$1" == "-d" ]; then
	# Start docker daemon

	# Drivers to use for Docker engines the tests are going to create.
	STORAGE_DRIVER=${STORAGE_DRIVER:-overlay}
	EXEC_DRIVER=${EXEC_DRIVER:-native}

	docker --daemon --log-level=panic \
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
fi

# If DOCKER_VOLUME is unset, create a temporary directory to cache containers
# between runs
# Only do this on Linux, because using /var/lib/docker from a host volume seems
# problematic with boot2docker.
if [ "$DOCKER_VOLUME" = "" -a `uname` = "Linux" ]; then
	volumes_template="`pwd`/docker-versions.XXXXX"
	volume=`mktemp -d "$volumes_template"`
	trap "rm -rf $tmpdir $volume" EXIT
else
	volume="$DOCKER_VOLUME"
fi

# Released versions

versions="1.6.0 1.6.1 1.7.0 1.7.1"

for v in $versions; do
	echo "Extracting Docker $v from dind image"
	binpath="$tmpdir/docker-$v/docker"
	ID=$(docker create dockerswarm/dind:$v)
	docker cp "$ID:/usr/local/bin/docker" "$tmpdir/docker-$v"

	echo "Running tests with Docker $v"
	DOCKER_BINARY="$binpath" DOCKER_VOLUME="$volume" ./run.sh

	# Cleanup.
	docker rm -f "$ID"
done

# Latest experimental version

echo "Extracting Docker master from dind image"
binpath="$tmpdir/docker-master/docker"
docker pull dockerswarm/dind-master
ID=$(docker create dockerswarm/dind-master)
docker cp "$ID:/usr/local/bin/docker" "$tmpdir/docker-master"

echo "Running tests with Docker master"
DOCKER_BINARY="$binpath" DOCKER_VOLUME="$volume" ./run.sh

# Cleanup.
docker rm -f "$ID"
