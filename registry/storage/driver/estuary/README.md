
### Build and Run lighthouse registry

1. Build the binary

```bash
$ make
```

2. Setup Config file (config.yml)

```yaml
version: 0.1
log:
  fields:
    service: registry

storage:
  redirect:
    disable: false
  
  estuary:
    url: https://api.estuary.tech
    shuttle-url: https://shuttle-4.estuary.tech
    auth-token: TOKEN_GOES_HERE
    rootdirectory: /Users/satoshi/registries/estuary-prod

http:
  addr: :5005
  headers:
    X-Content-Type-Options: [nosniff]
health:
  storagedriver:
    enabled: true
    interval: 10s
    threshold: 3

```

3. Run the service

```
$ bin/registry serve config.yml
WARN[0000] No HTTP secret provided - generated random secret. This may cause problems with uploads if multiple registries are behind a load-balancer. To provide a shared secret, fill in http.secret in the configuration file or set the REGISTRY_HTTP_SECRET environment variable.  go.version=go1.17.3 instance.id=fa0b0fca-3f04-41ac-8f95-02be502e8672 service=registry version=v2.7.0-1983-gefbad67e.m
INFO[0000] redis not configured                          go.version=go1.17.3 instance.id=fa0b0fca-3f04-41ac-8f95-02be502e8672 service=registry version=v2.7.0-1983-gefbad67e.m
INFO[0000] Starting upload purge in 17m0s                go.version=go1.17.3 instance.id=fa0b0fca-3f04-41ac-8f95-02be502e8672 service=registry version=v2.7.0-1983-gefbad67e.m
INFO[0000] listening on [::]:5005                        go.version=go1.17.3 instance.id=fa0b0fca-3f04-41ac-8f95-02be502e8672 service=registry version=v2.7.0-1983-gefbad67e.m
...
```

#### Configure 

- Create `Dockerfile`:

    ```dockerfile
    FROM busybox:latest
    CMD echo 'hello world'
    ```

- Build Docker image:

    ```bash
    $ docker build -t example/helloworld .
    ```

    Test run:

    ```bash
    $ docker run example/helloworld:latest
    hello world
    ```

    Tag and push
    ```
    $ docker tag example/helloworld registry-local:5005/helloworld:v1
    $ docker push registry-local:5005/helloworld:v1
    The push refers to repository [registry-local:5005/helloworld]
    eb6b01329ebe: Pushed 
    v1: digest: sha256:8c061639004b9506a38e11fad10ce1a6270207e80c4a50858fa22cd2c115b955 size: 526
    ```
    
- Pull image from lighthouse registry
    ```bash
    $ docker pull registry-local:5005/helloworld:v1
    v1: Pulling from helloworld
    Digest: sha256:8c061639004b9506a38e11fad10ce1a6270207e80c4a50858fa22cd2c115b955
    Status: Image is up to date for registry-local:5005/helloworld:v1
    ```

- Run image pulled from Lighthouse:

    ```bash
    $ docker run registry-local:5005/helloworld:v1
    hello world
    ```

### Transferring images from dockerhub to registry

Included is a helper script to move all tagged images from dockerhub, quay.io, etc. to the lighthouse
decentralized registry.

Below is an example call to pull and transfer all tagged alpine images

```
$ script/migrateimages alpine registry-local:5005
```