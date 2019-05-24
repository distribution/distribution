#!/usr/bin/env sh

set -eo pipefail


if [[ "$DOCKER_REG_AUTH_CERT" && "$REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE" ]]; then
    dir=$(dirname $REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE)
    mkdir -p $dir
    echo "------------------------------------------------"
    echo "[INFO] writing cert to $REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE"
    echo -e "${DOCKER_REG_AUTH_CERT}" > $REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE
else
    echo "------------------------------------------------"
    echo "[WARNING] you have not set env DOCKER_REG_AUTH_CERT or REGISTRY_AUTH_TOKEN_ROOTCERTBUNDLE"
    echo "------------------------------------------------"
fi

echo "[INFO] registry serve /etc/docker/registry/config.yml"
registry serve /etc/docker/registry/config.yml