---
description: Using systemd to manage registry container
keywords: registry, on-prem, systemd, socket-activated, recipe, advanced
title: Start registry via systemd
---

## Use-case

Using systemd to manage containers can make service discovery and maintenance easier
by managing all services in the same way. Additionally, when using Podman, systemd
can start the registry with socket-activation, providing additional security options:

* Run as non-root and expose on a low-numbered socket (< 1024)
* Run with `--network=none`

### Docker

When deploying the registry via Docker, a simple service file can be used to manage
the registry:

registry.service

```ini
[Unit]
Description=Distribution registry
After=docker.service
Requires=docker.service

[Service]
#TimeoutStartSec=0
Restart=always
ExecStartPre=-/usr/bin/docker stop %N
ExecStartPre=-/usr/bin/docker rm %N
ExecStart=/usr/bin/docker run --name %N \
    -v registry:/var/lib/registry \
    -p 5000:5000 \
    registry:2

[Install]
WantedBy=multi-user.target
```

In this case, the registry will store images in the named-volume `registry`.
Note that the container is destroyed on restart instead of using `--rm` or
destroy on stop. This is done to make accessing `docker logs ...` easier in
the case of issues.

### Podman

Podman offers tighter integration with systemd than Docker does, and supports
socket-activation of containers.

#### Create service file

```sh
podman create --name registry --network=none -v registry:/var/lib/registry registry:2
podman generate systemd --name --new registry > registry.service
```

#### Create socket file

registry.socket

```ini
[Unit]
Description=Distribution registry

[Socket]
ListenStream=5000

[Install]
WantedBy=sockets.target
```

### Installation

Installation can be either rootful or rootless. For Docker, rootless configurations
often include additional setup steps that are beyond the scope of this recipe, whereas
for Podman, rootless containers generally work out of the box.

#### Rootful

Run as root:

* Copy registry.service (and registry.socket if relevant) to /etc/systemd/service/
* Run `systemctl daemon-reload`
* Enable the service:
  * When using socket activation: `systemctl enable registry.socket`
  * When **not** using socket activation: `systemctl enable registry.service`
* Start the service:
  * When using socket activation: `systemctl start registry.socket`
  * When **not** using socket activation: `systemctl start registry.service`

#### Rootless

Run as the target user:

* Copy registry.service (and registry.socket if relevant) to ~/.config/systemd/user/
* Run `systemctl --user daemon-reload`
* Enable the service:
  * When using socket activation: `systemctl --user enable registry.socket`
  * When **not** using socket activation: `systemctl --user enable registry.service`
* Start the service:
  * When using socket activation: `systemctl --user start registry.socket`
  * When **not** using socket activation: `systemctl --user start registry.service`

**Note**: To have rootless services start on boot, it may be necessary to enable linger
via `loginctl enable-linger $USER`.
