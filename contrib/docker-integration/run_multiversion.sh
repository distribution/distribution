#!/usr/bin/env bash

# Run the integration tests with multiple versions of the Docker engine

set -e

# Don't use /tmp because this isn't available in boot2docker
tmpdir_template="`pwd`/docker-versions.XXXXX"
tmpdir=`mktemp -d "$tmpdir_template"`
trap "rm -rf $tmpdir" EXIT

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

versions="1.6.0 1.7.0"

for v in $versions; do
	echo "Downloading Docker $v"
	binpath="$tmpdir/docker-$v"
	curl -L -o "$binpath" "https://test.docker.com/builds/Linux/x86_64/docker-$v"
	chmod +x "$binpath"
	echo "Running tests with Docker $v"
	DOCKER_BINARY="$binpath" DOCKER_VOLUME="$volume" ./run.sh
done

# Latest experimental version

# Extract URI from https://experimental.docker.com/builds/
experimental=`curl -sSL https://experimental.docker.com/builds/ | tr " " "\n" | grep 'https://experimental.docker.com/builds/Linux/'`
echo "Downloading Docker experimental"
binpath="$tmpdir/docker-experimental"
curl -L -o "$binpath" "$experimental"
chmod +x "$binpath"
echo "Running tests with Docker experimental"
DOCKER_BINARY="$binpath" DOCKER_VOLUME="$volume" ./run.sh
