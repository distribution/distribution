page_title: Configure a Registry
page_description: Explains how to deploy a registry service
page_keywords: registry, service, images, repository


# Configure a Registry

The registry server can be configured with a YAML file. This section provides a
simple example and a complete reference.

## A simple development configuration

The following is a simple example that can used for local development:

```yaml
version: 0.1
log: 
	level: debug
storage:
    filesystem:
        rootdirectory: /tmp/registry-dev
http:
    addr: localhost:5000
    secret: asecretforlocaldevelopment
    debug:
        addr: localhost:5001
```

The above configures the registry instance to run on port 5000, binding to
"localhost", with the debug server enabled. Registry data will be stored in
"/tmp/registry-dev". Logging will be in "debug" mode, which is the most
verbose.

A similar simple configuration is available at
[config.yml](https://github.com/docker/distribution/blob/master/cmd/registry/config.yml), which is generally useful for local development.


##  Configuration Reference

Below is a comprehensive example of all possible configuration options for the registry. Some options are mutually exclusive, and each section is explained in more detail below, but this is a good starting point from which you may delete the sections you do not need to create your own configuration. A copy of this configuration can be found at config.sample.yml.

```yaml
version: 0.1
log:
	level: debug
	formatter: text
	fields:
		service: registry
		environment: staging
loglevel: debug # deprecated: use "log"
storage:
	filesystem:
		rootdirectory: /tmp/registry
	azure:
		accountname: accountname
		accountkey: base64encodedaccountkey
		container: containername
	s3:
		accesskey: awsaccesskey
		secretkey: awssecretkey
		region: us-west-1
		bucket: bucketname
		encrypt: true
		secure: true
		v4auth: true
		chunksize: 5242880
		rootdirectory: /s3/object/name/prefix
	cache:
		layerinfo: inmemory
auth:
	silly:
		realm: silly-realm
		service: silly-service
	token:
		realm: token-realm
		service: token-service
		issuer: registry-token-issuer
		rootcertbundle: /root/certs/bundle
middleware:
	registry:
		- name: ARegistryMiddleware
		  options:
			foo: bar
	repository:
		- name: ARepositoryMiddleware
		  options:
			foo: bar
	storage:
		- name: cloudfront
		  options:
			baseurl: https://my.cloudfronted.domain.com/
			privatekey: /path/to/pem
			keypairid: cloudfrontkeypairid
			duration: 3000
reporting:
	bugsnag:
		apikey: bugsnagapikey
		releasestage: bugsnagreleasestage
		endpoint: bugsnagendpoint
	newrelic:
		licensekey: newreliclicensekey
		name: newrelicname
http:
	addr: localhost:5000
	prefix: /my/nested/registry/
	secret: asecretforlocaldevelopment
	tls:
		certificate: /path/to/x509/public
		key: /path/to/x509/private
    clientcas:
      - /path/to/ca.pem
      - /path/to/another/ca.pem
	debug:
		addr: localhost:5001
notifications:
	endpoints: 
		- name: alistener
		  disabled: false
		  url: https://my.listener.com/event
		  headers: <http.Header>
		  timeout: 500
		  threshold: 5
		  backoff: 1000
redis:
	addr: localhost:6379
	password: asecret
	db: 0
	dialtimeout: 10ms
	readtimeout: 10ms
	writetimeout: 10ms
	pool:
		maxidle: 16
		maxactive: 64
		idletimeout: 300s
```

N.B. In some instances a configuration option may be marked **optional** but contain child options marked as **required**. This indicates that a parent may be omitted with all its children, however, if the parent is included, the children marked **required** must be included.

## version 

```yaml
version: 0.1
```

The version option is **required** and indicates the version of the configuration being used. It is expected to remain a top-level field, to allow for a consistent version check before parsing the remainder of the configuration file. 

N.B. The version of the registry software may be found at [/version/version.go](https://github.com/docker/distribution/blob/master/version/version.go)

## log

The log subsection configures the behavior of the logging system. The logging
system outputs everything to stdout. The granularity and format of the log
messages can be adjusted with this configuration section.

```yaml
log:
	level: debug
	formatter: text
	fields:
		service: registry
		environment: staging
```

- level: **Optional** - Sets the sensitivity of logging output. Permitted
  values are `error`, `warn`, `info` and `debug`. The default is `info`.
- formatter: **Optional** - This selects the format of logging output, which
  mostly affects how keyed attributes for a log line are encoded. Options are
  "text", "json" or "logstash". The default is "text".
- fields: **Optional** - A map of field names to values that will be added to
  every log line for the context. This is useful for identifying log messages
  source after being mixed in other systems.

## loglevel

> **DEPRECATED:** Please use [log](#log) instead.

```yaml
loglevel: debug
```

Permitted values are `error`, `warn`, `info` and `debug`. The default is
`info`.

## storage

```yaml
storage:
	filesystem:
		rootdirectory: /tmp/registry
	azure:
		accountname: accountname
		accountkey: base64encodedaccountkey
		container: containername
	s3:
		accesskey: awsaccesskey
		secretkey: awssecretkey
		region: us-west-1
		bucket: bucketname
		encrypt: true
		secure: true
		v4auth: true
		chunksize: 5242880
		rootdirectory: /s3/object/name/prefix
	cache:
		layerinfo: inmemory
```

The storage option is **required** and defines which storage backend is in use. At the moment only one backend may be configured, an error is returned when the registry is started with more than one storage backend configured.

A `cache` subsection can be used to enable caching of data accessed in the
storage backend. Currently, the only available cache provides fast access to
layer metadata. This if configured using the `layerinfo` field. The following
cache implementations are available:

- redis: using the redis pool to cache layer meta data.
- inmemory: use an in memory map to cache layer meta data.

The following backends may be configured, **all options for a given storage backend are required**:

### filesystem

This storage backend uses the local disk to store registry files. It is ideal for development and may be appropriate for some small scale production applications.

- rootdirectory: **Required** - This is the absolute path to directory in which the repository will store data.

### azure

This storage backend uses Microsoft's Azure Storage platform. 

- accountname: **Required** - Azure account name
- accountkey: **Required** - Azure account key
- container: **Required** - Name of the Azure container into which data will be stored

### S3

This storage backend uses Amazon's Simple Storage Service (a.k.a. S3).

- accesskey: **Required** - Your AWS Access Key
- secretkey: **Required** - Your AWS Secret Key.
- region: **Required** - The AWS region in which your bucket exists. For the moment, the Go AWS library in use does not use the newer DNS based bucket routing.
- bucket: **Required** - The bucket name in which you want to store the registry's data.
- encrypt: TODO: fill in description
- secure: TODO: fill in description
- v4auth: This indicates whether Version 4 of AWS's authentication should be used. Generally you will want to set this to true.
- chunksize: TODO: fill in description
- rootdirectory: **Optional** - This is a prefix that will be applied to all S3 keys to allow you to segment data in your bucket if necessary.

## auth

```yaml
auth:
	silly:
		realm: silly-realm
		service: silly-service
	token:
		realm: token-realm
		service: token-service
		issuer: registry-token-issuer
		rootcertbundle: /root/certs/bundle
```

The auth option is **optional** as there are use cases (i.e. a mirror that only permits pulls) for which authentication may not be desired. There are currently 2 possible auth providers, "silly" and "token", only one auth provider may be configured at the moment:

### silly

The "silly" auth is only for development purposes. It simply checks for the existence of the "Authorization" header in the HTTP request, with no regard for the value of the header. If the header does not exist, it will respond with a challenge response, echoing back the realm, service, and scope that access was denied for. 

The values of the ```realm``` and ```service``` options are used in authentication reponses, both options are **required**

- realm: **Required** - The realm in which the registry server authenticates.
- service: **Required** - The service being authenticated.

### token

Token based authentication allows the authentication system to be decoupled from the registry. It is a well established authentication paradigm with a high degree of security. 

- realm: **Required** - The realm in which the registry server authenticates.
- service: **Required** - The service being authenticated.
- issuer: **Required** - The name of the token issuer. The issuer inserts this into the token so it must match the value configured for the issuer.
- rootcertbundle: **Required** - The absolute path to the root certificate bundle containing the public part of the certificates that will be used to sign authentication tokens.

For more information about Token based authentication configuration, see the [specification.]

## middleware

The middleware option is **optional** and allows middlewares to be injected at named hook points. A requirement of all middlewares is that they implement the same interface as the object they're wrapping. This means a registry middleware must implement the `distribution.Registry` interface, repository middleware must implement `distribution.Respository`, and storage middleware must implement `driver.StorageDriver`.

Currently only one middleware, cloudfront, a storage middleware, is included in the registry. 

```yaml
middleware:
	registry:
		- name: ARegistryMiddleware
		  options:
			foo: bar
	repository:
		- name: ARepositoryMiddleware
		  options:
			foo: bar
	storage:
		- name: cloudfront
		  options:
			baseurl: https://my.cloudfronted.domain.com/
			privatekey: /path/to/pem
			keypairid: cloudfrontkeypairid
			duration: 3000
```

Each middleware entry has `name` and `options` entries. The `name` must correspond to the name under which the middleware registers itself. The `options` field is a map that details custom configuration required to initialize the middleware. It is treated as a map[string]interface{} and as such will support any interesting structures desired, leaving it up to the middleware initialization function to best determine how to handle the specific interpretation of the options.

### cloudfront

- baseurl: **Required** - SCHEME://HOST[/PATH] at which Cloudfront is served.
- privatekey: **Required** - Private Key for Cloudfront provided by AWS
- keypairid: **Required** - Key Pair ID provided by AWS
- duration: **Optional** - Duration for which a signed URL should be valid

## reporting

```yaml
reporting:
	bugsnag:
		apikey: bugsnagapikey
		releasestage: bugsnagreleasestage
		endpoint: bugsnagendpoint
	newrelic:
		licensekey: newreliclicensekey
		name: newrelicname
```

The reporting option is **optional** and configures error and metrics reporting tools. At the moment only two services are supported, New Relic and Bugsnag, a valid configuration may contain both.

### bugsnag

- apikey: **Required** - API Key provided by Bugsnag
- releasestage: **Optional** - TODO: fill in description
- endpoint: **Optional** - TODO: fill in description

### newrelic

- licensekey: **Required** - License key provided by New Relic
- name: **Optional** - New Relic application name

## http

```yaml
http:
	addr: localhost:5000
	prefix: /my/nested/registry/
	secret: asecretforlocaldevelopment
	tls:
		certificate: /path/to/x509/public
		key: /path/to/x509/private
    clientcas:
      - /path/to/ca.pem
      - /path/to/another/ca.pem
	debug:
		addr: localhost:5001
```

The http option details the configuration for the HTTP server that hosts the registry.

- addr: **Required** - The HOST:PORT for which the server should accept connections.
- prefix: **Optional** - If the server will not run at the root path, this should specify the prefix (the part of the path before ```v2```). It should have both preceding and trailing slashes.
- secret: A random piece of data. It is used to sign state that may be stored with the client to protect against tampering. For production use you should generate a random piece of data using a cryptographically secure random generator.

### tls

The tls option within http is **optional** and allows you to configure SSL for the server. If you already have a server such as Nginx or Apache running on the same host as the registry, you may prefer to configure SSL termination there and proxy connections to the registry server.

- certificate: **Required** - Absolute path to x509 cert file
- key: **Required** - Absolute path to x509 private key file
- clientcas: **Optional** - An array of absolute paths to a x509 CA file

### debug

The debug option is **optional** and allows you to configure a debug server that can be helpful in diagnosing problems. It is of most use to contributers to the distribution repository and should generally be disabled in production deployments.

- addr: **Required** - The HOST:PORT on which the debug server should accept connections.


## notifications

```yaml
notifications:
	endpoints: 
		- name: alistener
		  disabled: false
		  url: https://my.listener.com/event
		  headers: <http.Header>
		  timeout: 500
		  threshold: 5
		  backoff: 1000
```

The notifications option is **optional** and currently may contain a single option, ```endpoints```.

### endpoints

Endpoints is a list of named services (URLs) that can accept event notifications.

- name: **Required** - A human readable name for the service. 
- disabled: **Optional** - A boolean to enable/disable notifications for a service.
- url: **Required** - The URL to which events should be published.
- headers: **Required** - TODO: fill in description
- timeout: **Required** - TODO: fill in description
- threshold: **Required** - TODO: fill in description
- backoff: **Required** - TODO: fill in description

## redis

```yaml
redis:
	addr: localhost:6379
	password: asecret
	db: 0
	dialtimeout: 10ms
	readtimeout: 10ms
	writetimeout: 10ms
	pool:
		maxidle: 16
		maxactive: 64
		idletimeout: 300s
```

Declare parameters for constructing the redis connections. Registry instances
may use the redis instance for several applications. The current purpose is
caching information about immutable blobs. Most of the options below control
how the registry connects to redis. The behavior of the pool can be controlled
with the [pool](#pool) subsection.

- addr: **Required** - Address (host and port) of redis instance.
- password: **Optional** - A password used to authenticate to the redis instance.
- db: **Optional** - Selects the db for each connection.
- dialtimeout: **Optional** - Timeout for connecting to a redis instance.
- readtimeout: **Optional** - Timeout for reading from redis connections.
- writetimeout: **Optional** - Timeout for writing to redis connections.

### pool

```yaml
pool:
	maxidle: 16
	maxactive: 64
	idletimeout: 300s
```

Configure the behavior of the redis connection pool.

- maxidle: **Optional** - sets the maximum number of idle connections.
- maxactive: **Optional** - sets the maximum number of connections that should
  be opened before blocking a connection request.
- idletimeout: **Optional** - sets the amount time to wait before closing
  inactive connections.
