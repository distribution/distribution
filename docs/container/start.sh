#!/bin/sh

echo "[starter] starting..."

# Fail hard and fast
set -eo pipefail

# If this fails, docker will restart the container. Yay, docker.
confd -node https://dtr-etcd-${DTR_REPLICA_ID}.dtr-br:2379 -node https://dtr-etcd-${DTR_REPLICA_ID}.dtr-br:4001 -onetime -config-file /etc/confd/confd.toml

# Run confd watcher in the background to watch the upstream servers
confd -node https://dtr-etcd-${DTR_REPLICA_ID}.dtr-br:2379 -node https://dtr-etcd-${DTR_REPLICA_ID}.dtr-br:4001 -config-file /etc/confd/confd.toml &
echo "[starter] confd is listening for changes on etcd..."

# Start registry
echo "[starter] starting registry service..."
while true
do
    /bin/registry || true
    sleep 1
done
