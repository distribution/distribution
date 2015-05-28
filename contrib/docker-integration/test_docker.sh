#!/bin/sh

hostname=$1
if [ "$hostname" = "" ]; then
	hostname="localhost"
fi

docker pull hello-world

# TLS Configuration chart
# Username/Password: testuser/passpassword
#      | ca  | client | basic | notes
# 5440 | yes | no     | no    | Tests CA certificate
# 5441 | yes | no     | yes   | Tests basic auth over TLS
# 5442 | yes | yes    | no    | Tests client auth with client CA
# 5443 | yes | yes    | no    | Tests client auth without client CA
# 5444 | yes | yes    | yes   | Tests using basic auth + tls auth
# 5445 | no  | no     | no    | Tests insecure using TLS
# 5446 | no  | no     | yes   | Tests sending credentials to server with insecure TLS
# 5447 | no  | yes    | no    | Tests client auth to insecure
# 5448 | yes | no     | no    | Bad SSL version
docker tag -f hello-world $hostname:5440/hello-world
docker push $hostname:5440/hello-world
if [ $? -ne 0 ]; then
	echo "Fail to push"
	exit 1
fi

docker login -u testuser -p passpassword -e distribution@docker.com $hostname:5441
if [ $? -ne 0 ]; then
	echo "Failed to login"
	exit 1
fi
docker tag -f hello-world $hostname:5441/hello-world
docker push $hostname:5441/hello-world
if [ $? -ne 0 ]; then
	echo "Fail to push"
	exit 1
fi

docker tag -f hello-world $hostname:5442/hello-world
docker push $hostname:5442/hello-world
if [ $? -ne 0 ]; then
	echo "Fail to push"
	exit 1
fi

docker tag -f hello-world $hostname:5443/hello-world
docker push $hostname:5443/hello-world
if [ $? -eq 0 ]; then
	echo "Expected failure"
	exit 1
fi

docker login -u testuser -p passpassword -e distribution@docker.com $hostname:5444
if [ $? -ne 0 ]; then
	echo "Failed to login"
	exit 1
fi
docker tag -f hello-world $hostname:5444/hello-world
docker push $hostname:5444/hello-world
if [ $? -ne 0 ]; then
	echo "Fail to push"
	exit 1
fi

docker tag -f hello-world $hostname:5445/hello-world
docker push $hostname:5445/hello-world
if [ $? -eq 0 ]; then
	echo "Expected failure with insecure registry"
	exit 1
fi

docker login -u testuser -p passpassword -e distribution@docker.com $hostname:5446
if [ $? -ne 0 ]; then
	echo "Failed to login"
	exit 1
fi
docker tag -f hello-world $hostname:5446/hello-world
docker push $hostname:5446/hello-world
if [ $? -eq 0 ]; then
	echo "Expected failure with insecure registry"
	exit 1
fi

docker tag -f hello-world $hostname:5447/hello-world
docker push $hostname:5447/hello-world
if [ $? -eq 0 ]; then
	echo "Expected failure with insecure registry"
	exit 1
fi

docker tag -f hello-world $hostname:5448/hello-world
docker push $hostname:5448/hello-world
if [ $? -eq 0 ]; then
	echo "Expected failure contacting with sslv3"
	exit 1
fi
