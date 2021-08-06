---
description: Deploying a Registry in an insecure fashion
keywords: registry, on-prem, images, tags, repository, distribution, insecure
title: Test an insecure registry
---

{% include registry.md %}

While it's highly recommended to secure your registry using a TLS certificate
issued by a known CA, you can choose to use self-signed certificates, or use
your registry over an unencrypted HTTP connection. Either of these choices
involves security trade-offs and additional configuration steps.

## Deploy a plain HTTP registry

> **Warning**:
> It's not possible to use an insecure registry with basic authentication.
{:.warning}

This procedure configures Docker to entirely disregard security for your
registry. This is **very** insecure and is not recommended. It exposes your
registry to trivial man-in-the-middle (MITM) attacks. Only use this solution for
isolated testing or in a tightly controlled, air-gapped environment.

1.  Edit the `daemon.json` file, whose default location is
    `/etc/docker/daemon.json` on Linux or
    `C:\ProgramData\docker\config\daemon.json` on Windows Server. If you use
    Docker Desktop for Mac or Docker Desktop for Windows, click the Docker icon, choose
    **Preferences** (Mac) or **Settings** (Windows), and choose **Docker Engine**.

    If the `daemon.json` file does not exist, create it. Assuming there are no
    other settings in the file, it should have the following contents:

    ```json
    {
      "insecure-registries" : ["myregistrydomain.com:5000"]
    }
    ```

    Substitute the address of your insecure registry for the one in the example.

    With insecure registries enabled, Docker goes through the following steps:

    - First, try using HTTPS.
      - If HTTPS is available but the certificate is invalid, ignore the error
        about the certificate.
      - If HTTPS is not available, fall back to HTTP.


2. Restart Docker for the changes to take effect.


Repeat these steps on every Engine host that wants to access your registry.


## Use self-signed certificates

> **Warning**:
> Using this along with basic authentication requires to **also** trust the certificate into the OS cert store for some versions of docker (see below)
{:.warning}

This is more secure than the insecure registry solution.

1.  Generate your own certificate:

    ```console
    $ mkdir -p certs

    $ openssl req \
      -newkey rsa:4096 -nodes -sha256 -keyout certs/domain.key \
      -addext "subjectAltName = DNS:myregistry.domain.com" \
      -x509 -days 365 -out certs/domain.crt
    ```

    Be sure to use the name `myregistrydomain.com` as a CN.

2.  Use the result to [start your registry with TLS enabled](./deploying.md#get-a-certificate).

3.  Instruct every Docker daemon to trust that certificate. The way to do this
    depends on your OS.

    - **Linux**: Copy the `domain.crt` file to
      `/etc/docker/certs.d/myregistrydomain.com:5000/ca.crt` on every Docker
      host. You do not need to restart Docker.

    - **Windows Server**:

      1.  Open Windows Explorer, right-click the `domain.crt`
          file, and choose Install certificate. When prompted, select the following
          options:

          | Store location                                | local machine |
          | Place all certificates in the following store | selected      |

      2.  Click **Browser** and select **Trusted Root Certificate Authorities**.

      3.  Click **Finish**. Restart Docker.

    - **Docker Desktop for Mac**: Follow the instructions in
      [Adding custom CA certificates](../docker-for-mac/index.md#add-tls-certificates){: target="_blank" rel="noopener" class="_"}.
      Restart Docker.

    - **Docker Desktop for Windows**: Follow the instructions in
      [Adding custom CA certificates](../docker-for-windows/index.md#adding-tls-certificates){: target="_blank" rel="noopener" class="_"}.
      Restart Docker.


## Troubleshoot insecure registry

This section lists some common failures and how to recover from them.

### Failing...

Failing to configure the Engine daemon and trying to pull from a registry that is not using
TLS results in the following message:

```none
FATA[0000] Error response from daemon: v1 ping attempt failed with error:
Get https://myregistrydomain.com:5000/v1/_ping: tls: oversized record received with length 20527.
If this private registry supports only HTTP or HTTPS with an unknown CA certificate, add
`--insecure-registry myregistrydomain.com:5000` to the daemon's arguments.
In the case of HTTPS, if you have access to the registry's CA certificate, no need for the flag;
simply place the CA certificate at /etc/docker/certs.d/myregistrydomain.com:5000/ca.crt
```

### Docker still complains about the certificate when using authentication?

When using authentication, some versions of Docker also require you to trust the
certificate at the OS level.

#### Ubuntu

```console
$ cp certs/domain.crt /usr/local/share/ca-certificates/myregistrydomain.com.crt
update-ca-certificates
```

#### Red Hat Enterprise Linux

```console
$ cp certs/domain.crt /etc/pki/ca-trust/source/anchors/myregistrydomain.com.crt
update-ca-trust
```

#### Oracle Linux

```console
$ update-ca-trust enable
```

Restart Docker for the changes to take effect.

### Windows

Open Windows Explorer, right-click the certificate, and choose
**Install certificate**.

Then, select the following options:

* Store location: local machine
* Check **place all certificates in the following store**
* Click **Browser**, and select **Trusted Root Certificate Authorities**
* Click **Finish**

[Learn more about managing TLS certificates](https://technet.microsoft.com/en-us/library/cc754841(v=ws.11).aspx#BKMK_addlocal).

After adding the CA certificate to Windows, restart Docker Desktop for Windows.
