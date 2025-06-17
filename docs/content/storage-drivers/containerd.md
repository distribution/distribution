---
title: Containerd Storage Driver
description: Explains how to configure and use the containerd storage driver.
---

The `containerd` storage driver allows the registry to use a [containerd](https://containerd.io/) instance for storing image layers and metadata. This can be useful in environments where containerd is already running and managing container images.

## Configuration

To use the `containerd` storage driver, you need to configure the `storage` section in your registry's `config.yml` file.

```yaml
storage:
  driver: containerd
  containerd:
    address: /run/containerd/containerd.sock  # Optional: defaults to this path
    namespace: default                       # Optional: defaults to "default"
    # root: /var/lib/registry-containerd      # Optional: path within containerd's metadata store, defaults to "/registry"
    # contentdir: /var/lib/containerd/io.containerd.content.v1.content # Optional: if containerd's content store is not in the default location
```

### Parameters

-   `address`: (Optional) The path to the containerd gRPC socket.
    -   Default: `/run/containerd/containerd.sock`
    -   This can also be set using the `REGISTRY_STORAGE_CONTAINERD_ADDRESS` environment variable.
-   `namespace`: (Optional) The containerd namespace to use for storing registry data.
    -   Default: `default`
    -   This can also be set using the `REGISTRY_STORAGE_CONTAINERD_NAMESPACE` environment variable.
-   `root`: (Optional) The root path within containerd's metadata store where registry data will be stored. This is a conceptual path used by the driver and does not directly translate to a host filesystem path for all data.
    -   Default: `/registry`
    -   This can also be set using the `REGISTRY_STORAGE_CONTAINERD_ROOT` environment variable.
-   `contentdir`: (Optional) The path to containerd's content store directory. This should only be set if your containerd instance uses a non-standard location for its content store. The registry needs access to this directory.
    -   Default: The driver will attempt to determine this from containerd's configuration or use common defaults.
    -   This can also be set using the `REGISTRY_STORAGE_CONTAINERD_CONTENTDIR` environment variable.


## Usage

Once configured, the registry will automatically use the containerd instance for all storage operations. Ensure that the user running the registry process has the necessary permissions to access the containerd socket and, if specified, the `contentdir`.

### Permissions

The user running the Docker Registry process must have read and write permissions to the containerd socket (e.g., by being in the `docker` or `containerd` group, or through specific ACLs). If `contentdir` is explicitly set, the registry user also needs appropriate access to that directory.

### Data Storage

Image layers will be stored in containerd's content store, and metadata related to these layers (manifests, tags, etc.) will be managed by the driver, typically using labels or other mechanisms within containerd's metadata store under the configured `namespace` and `root`.

### Important Considerations

-   **Experimental:** This driver is currently experimental. Use with caution in production environments.
-   **Containerd Version:** It's recommended to use a recent and stable version of containerd.
-   **Resource Management:** Storage space used by the registry will be managed by containerd. Monitor containerd's disk usage accordingly.
-   **Shared Store:** If the containerd instance is also used by other services (e.g., Kubernetes, local `ctr` commands), be aware that registry data will reside alongside other container images and content. Ensure proper namespacing and resource allocation.
-   **Backup and Restore:** Backing up registry data stored via the containerd driver involves backing up the relevant parts of the containerd storage. Refer to containerd's documentation for best practices on backing up and restoring its state.

## Cleaning up unused data

The containerd storage driver itself does not implement a separate garbage collection mechanism. It relies on containerd's own garbage collection capabilities. You can use `ctr content garbage-collect` or similar containerd tooling to remove unreferenced content. Ensure that no other services are relying on the content before manually deleting it.

When blobs are "deleted" by the registry (e.g., via the API), the driver will remove its references to these blobs within its metadata. However, the actual blob data in containerd's content store will only be removed when containerd's garbage collection runs and identifies them as unreferenced.
