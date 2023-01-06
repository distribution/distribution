#!/bin/sh

set +e

TIMEOUT=5
while [ $TIMEOUT -gt 0 ]; do
    STATUS=$(curl --insecure -s -o /dev/null -w '%{http_code}' http://localhost:5001/debug/health)
    echo $STATUS
    if [ $STATUS -eq 200 ]; then
		    break
    fi
    TIMEOUT=$(($TIMEOUT - 1))
    sleep 5
done

if [ $TIMEOUT -eq 0 ]; then
    echo "Distribution cannot be available within one minute."
    exit 1
fi

set -e

docker pull hello-world:latest
docker tag hello-world:latest $1:5000/distribution/hello-world:latest
docker push $1:5000/distribution/hello-world:latest
docker pull $1:5000/distribution/hello-world:latest