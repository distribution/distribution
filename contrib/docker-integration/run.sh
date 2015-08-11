#!/usr/bin/env bash
set -e
set -x

source helpers.bash

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

# Port used by engine under test
ENGINE_PORT=5216

# Root directory of Distribution
DISTRIBUTION_ROOT=$(cd ../..; pwd -P)

DOCKER_GRAPHDRIVER=${DOCKER_GRAPHDRIVER:-overlay}
EXEC_DRIVER=${EXEC_DRIVER:-native}

volumeMount=""
if [ "$DOCKER_VOLUME" != "" ]; then
	volumeMount="-v ${DOCKER_VOLUME}:/var/lib/docker"
fi

dockerMount=""
if [ "$DOCKER_BINARY" != "" ]; then
	dockerMount="-v ${DOCKER_BINARY}:/usr/local/bin/docker"
else
	DOCKER_BINARY=docker
fi

# Image containing the integration tests environment.
INTEGRATION_IMAGE=${INTEGRATION_IMAGE:-distribution/docker-integration}

if [ "$1" == "-d" ]; then
	start_daemon
	shift
fi

TESTS=${@:-.}

# Make sure we upgrade the integration environment.
docker pull $INTEGRATION_IMAGE

# Start a Docker engine inside a docker container
ID=$(docker run -d -it -p $ENGINE_PORT:$ENGINE_PORT --privileged $volumeMount $dockerMount \
	-v ${DISTRIBUTION_ROOT}:/go/src/github.com/docker/distribution \
	-e "ENGINE_PORT=$ENGINE_PORT" \
	-e "DOCKER_GRAPHDRIVER=$DOCKER_GRAPHDRIVER" \
	-e "EXEC_DRIVER=$EXEC_DRIVER" \
	${INTEGRATION_IMAGE} \
	./run_engine.sh)

# Wait for it to become reachable.
tries=10
until "$DOCKER_BINARY" -H "127.0.0.1:$ENGINE_PORT" version &> /dev/null; do
	(( tries-- ))
	if [ $tries -le 0 ]; then
		echo >&2 "error: daemon failed to start"
		exit 1
	fi
	sleep 1
done

# Make sure we have images outside the container, to transfer to the container.
# Not much will happen here if the images are already present.
docker-compose pull
docker-compose build

# Transfer images to the inner container.
for image in "$INTEGRATION_IMAGE" registry:0.9.1 dockerintegration_nginx dockerintegration_registryv2; do
	docker save "$image" | "$DOCKER_BINARY" -H "127.0.0.1:$ENGINE_PORT" load
done

#DOCKER_HOST="tcp://127.0.0.1:$ENGINE_PORT" docker-compose pull
#DOCKER_HOST="tcp://127.0.0.1:$ENGINE_PORT" docker-compose build

# Run the tests.
docker exec -it "$ID" sh -c "DOCKER_HOST=tcp://127.0.0.1:$ENGINE_PORT ./test_runner.sh $TESTS"

# Stop container
docker rm -f -v "$ID"
