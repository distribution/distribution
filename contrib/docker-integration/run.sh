#!/usr/bin/env bash
set -e

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

# Root directory of Distribution
DISTRIBUTION_ROOT=$(cd ../..; pwd -P)

volumeMount=""
if [ "$DOCKER_VOLUME" != "" ]; then
	volumeMount="-v ${DOCKER_VOLUME}:/var/lib/docker"
fi

dockerMount=""
if [ "$DOCKER_BINARY" != "" ]; then
	dockerMount="-v ${DOCKER_BINARY}:/usr/local/bin/docker"
fi

# Image containing the integration tests environment.
INTEGRATION_IMAGE=${INTEGRATION_IMAGE:-distribution/docker-integration}

# Make sure we upgrade the integration environment.
docker pull $INTEGRATION_IMAGE

# Start the integration tests in a Docker container.
ID=$(docker run -d -t --privileged $volumeMount $dockerMount \
	-v ${DISTRIBUTION_ROOT}:/go/src/github.com/docker/distribution \
	-e "STORAGE_DRIVER=$DOCKER_GRAPHDRIVER" \
	-e "EXEC_DRIVER=$EXEC_DRIVER" \
	${INTEGRATION_IMAGE} \
	./test_runner.sh "$@")

# Clean it up when we exit.
trap "docker rm -f -v $ID > /dev/null" EXIT

docker logs -f $ID
