<!--GITHUB
page_title: Configure a Registry
page_description: Explains how to deploy a registry 
page_keywords: registry, service, images, repository
IGNORES-->


# Registry Configuration Reference

You configure a registry server using a YAML file. This page explains the
configuration options and the values they can take. You'll also find examples of
middleware and development environment configurations.

## Overriding configuration options
Environment variables may be used to override configuration parameters other than 
version.  To override a configuration option, create an environment variable named 
REGISTRY\_variable_ where *variable* is the name of the configuration option.

e.g 
```
REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/tmp/registry/test
```

will set the storage root directory to `/tmp/registry/test`

## List of configuration options

This section lists all the registry configuration options. Some options in
the list are mutually exclusive. So, make sure to read the detailed reference
information about each option that appears later in this page.

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
	maintenance:
		uploadpurging:
			enabled: true
			age: 168h
			interval: 24h
			dryrun: false
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
		verbose: true
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

In some instances a configuration option is **optional** but it contains child
options marked as **required**. This indicates that you can omit the parent with
all its children. However, if the parent is included, you must also include all
the children marked **required**.

## Override configuration options

You can use environment variables to override most configuration parameters. The
exception is the `version` variable which cannot be overridden. You can set
environment variables on the command line using the `-e` flag on `docker run` or
from within a Dockerfile using the `ENV` instruction.

To override a configuration option, create an environment variable named
`REGISTRY\variable_` where *`variable`* is the name of the configuration option
and the `_` (underscore) represents indention levels. For example, you can
configure the `rootdirectory` of the `filesystem` storage backend:

```
storage:
	filesystem:
		rootdirectory: /tmp/registry
```

To override this value, set an environment variable like this:

```
REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/tmp/registry/test
```

This variable overrides the `/tmp/registry` value to the `/tmp/registry/test`
directory.

>**Note**: If an environment variable changes a map value into a string, such
>as replacing the storage driver type with `REGISTRY_STORAGE=filesystem`, then
>all sub-fields will be erased. As such, specifying the storage type in the
>environment will remove all parameters related to the old storage
>configuration.


## version 

```yaml
version: 0.1
```

The `version` option is **required**. It specifies the configuration's version.
It is expected to remain a top-level field, to allow for a consistent version
check before parsing the remainder of the configuration file. 

## log

The `log` subsection configures the behavior of the logging system. The logging
system outputs everything to stdout. You can adjust the granularity and format
with this configuration section.

```yaml
log:
	level: debug
	formatter: text
	fields:
		service: registry
		environment: staging
```

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>level</code>
    </td>
    <td>
      no
    </td>
    <td>
      Sets the sensitivity of logging output. Permitted values are
      <code>error</code>, <code>warn</code>, <code>info</code> and
      <code>debug</code>. The default is <code>info</code>.
    </td>
  </tr>
  <tr>
    <td>
      <code>formatter</code>
    </td>
    <td>
      no
    </td>
    <td>
      This selects the format of logging output. The format primarily affects how keyed
      attributes for a log line are encoded. Options are <code>text</code>, <code>json</code> or
      <code>logstash</code>. The default is <code>text</code>.
    </td>
  </tr>
    <tr>
    <td>
      <code>fields</code>
    </td>
    <td>
      no
    </td>
    <td>
      A map of field names to values. These are added to every log line for
      the context. This is useful for identifying log messages source after
      being mixed in other systems.
    </td>
</table>


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
	maintenance:
		uploadpurging:
			enabled: true
			age: 168h
			interval: 24h
			dryrun: false
```

The storage option is **required** and defines which storage backend is in use.
You must configure one backend; if you configure more, the registry returns an error.

### cache

Use the `cache` subsection to enable caching of data accessed in the storage
backend. Currently, the only available cache provides fast access to layer
metadata. This, if configured, uses the `layerinfo` field.  

You can set `layerinfo` field to `redis` or `inmemory`.  The `redis` value uses
a Redis pool to cache layer metadata.  The `inmemory` value uses an in memory
map.

### filesystem

The `filesystem` storage backend uses the local disk to store registry files. It
is ideal for development and may be appropriate for some small-scale production
applications.

This backend has a single, required `rootdirectory` parameter. The parameter
specifies the absolute path to a directory. The registry stores all its data
here so make sure there is adequate space available.

### azure

This storage backend uses Microsoft's Azure Storage platform. 

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>accountname</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Azure account name.
    </td>
  </tr>
   <tr>
    <td>
      <code>accountkey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Azure account key.
    </td>
  </tr>
   <tr>
    <td>
      <code>container</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Name of the Azure container into which to store data.
    </td>
  </tr>  
</table>



### S3

This storage backend uses Amazon's Simple Storage Service (S3).

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>accesskey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Your AWS Access Key.
    </td>
  </tr>
    <tr>
    <td>
      <code>secretkey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Your AWS Secret Key.
    </td>
  </tr>
    <tr>
    <td>
      <code>region</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The AWS region in which your bucket exists. For the moment, the Go AWS
      library in use does not use the newer DNS based bucket routing.
    </td>
  </tr>
    <tr>
    <td>
      <code>bucket</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The bucket name in which you want to store the registry's data.
    </td>
  </tr>
    <tr>
    <td>
      <code>encrypt</code>
    </td>
    <td>
      no
    </td>
    <td>
       Specifies whether the registry stores the image in encrypted format or
       not. A boolean value. The default is false.
    </td>
  </tr>
    <tr>
    <td>
      <code>secure</code>
    </td>
    <td>
      no
    </td>
    <td>
      Indicates whether to use HTTPS instead of HTTP. A boolean value. The
      default is false.
    </td>
  </tr>
    <tr>
    <td>
      <code>v4auth</code>
    </td>
    <td>
      no
    </td>
    <td>
      Indicates whether the registry uses Version 4 of AWS's authentication.
      Generally, you should set this to <code>true</code>. By default, this is
      <code>false</code>.
    </td>
  </tr>
    <tr>
    <td>
      <code>chunksize</code>
    </td>
    <td>
      no
    </td>
    <td>
      The S3 API requires multipart upload chunks to be at least 5MB. This value
      should be a number that is larger than 5*1024*1024.
    </td>
  </tr>
   <tr>
    <td>
      <code>rootdirectory</code>
    </td>
    <td>
      no
    </td>
    <td>
      This is a prefix that will be applied to all S3 keys to allow you to segment data in your bucket if necessary.
    </td>
  </tr> 
</table>

### Maintenance

Currently the registry can perform one maintenance function: upload purging.  This and future
maintenance functions which are related to storage can be configured under the maintenance section.

### Upload Purging

Upload purging is a background process that periodically removes orphaned files from the upload
directories of the registry.  Upload purging is enabled by default.  To 
 configure upload directory purging, the following parameters
must be set.


| Parameter | Required | Description
  --------- | -------- | -----------
`enabled` | yes | Set to true to enable upload purging.  Default=true. |
`age` | yes | Upload directories which are older than this age will be deleted.  Default=168h (1 week)
`interval` | yes | The interval between upload directory purging.  Default=24h.  
`dryrun` | yes |  dryrun can be set to true to obtain a summary of what directories will be deleted.  Default=false.

Note: `age` and `interval` are strings containing a number with optional fraction and a unit suffix: e.g. 45m, 2h10m, 168h (1 week).  

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

The `auth` option is **optional** as there are use cases (i.e. a mirror that
only permits pulls) for which authentication may not be desired. There are
currently 2 possible auth providers, `silly` and `token`. You can configure only
one `auth` provider.

### silly

The `silly` auth is only for development purposes. It simply checks for the
existence of the `Authorization` header in the HTTP request. It has no regard for
the header's value. If the header does not exist, the `silly` auth responds with a
challenge response, echoing back the realm, service, and scope that access was
denied for. 

The following values are used to configure the response:

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>realm</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The realm in which the registry server authenticates.
    </td>
  </tr>
    <tr>
    <td>
      <code>service</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The service being authenticated.
    </td>
  </tr>
</table>



### token

Token based authentication allows the authentication system to be decoupled from
the registry. It is a well established authentication paradigm with a high
degree of security. 

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>realm</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The realm in which the registry server authenticates.
    </td>
  </tr>
    <tr>
    <td>
      <code>service</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The service being authenticated.
    </td>
  </tr>
    <tr>
    <td>
      <code>issuer</code>
    </td>
    <td>
      yes
    </td>
    <td>
The name of the token issuer. The issuer inserts this into
the token so it must match the value configured for the issuer.
    </td>
  </tr>
    <tr>
    <td>
      <code>rootcertbundle</code>
    </td>
    <td>
			yes 
     </td>
    <td>
The absolute path to the root certificate bundle. This bundle contains the
public part of the certificates that is used to sign authentication tokens.
     </td>
  </tr>
</table> 

For more information about Token based authentication configuration, see the [specification.]

## middleware

The `middleware` option is **optional**. Use this option to inject middleware at
named hook points. All middlewares must implement the same interface as the
object they're wrapping. This means a registry middleware must implement the
`distribution.Namespace` interface, repository middleware must implement
`distribution.Respository`, and storage middleware must implement
`driver.StorageDriver`.

Currently only one middleware, `cloudfront`, a storage middleware, is supported
in the registry implementation. 

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

Each middleware entry has `name` and `options` entries. The `name` must
correspond to the name under which the middleware registers itself. The
`options` field is a map that details custom configuration required to
initialize the middleware. It is treated as a `map[string]interface{}`. As such,
it supports any interesting structures desired, leaving it up to the middleware
initialization function to best determine how to handle the specific
interpretation of the options.

### cloudfront

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>baseurl</code>
    </td>
    <td>
      yes
    </td>
    <td>
      <code>SCHEME://HOST[/PATH]</code> at which Cloudfront is served.
    </td>
  </tr>
    <tr>
    <td>
      <code>privatekey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Private Key for Cloudfront provided by AWS.
    </td>
  </tr>
    <tr>
    <td>
      <code>keypairid</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Key pair ID provided by AWS.
    </td>
  </tr>
    <tr>
    <td>
      <code>duration</code>
    </td>
    <td>
      no
    </td>
    <td>
      Duration for which a signed URL should be valid.
    </td>
  </tr>
</table>


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
		verbose: true
```

The `reporting` option is **optional** and configures error and metrics
reporting tools. At the moment only two services are supported, [New
Relic](http://newrelic.com/) and [Bugsnag](http://bugsnag.com), a valid
configuration may contain both.

### bugsnag

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>apikey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      API Key provided by Bugsnag
    </td>
  </tr>
  <tr>
    <td>
      <code>releasestage</code>
    </td>
    <td>
      no
    </td>
    <td>
      Tracks where the registry is deployed, for example,
      <codde>production</code>,<codde>staging</code>, or
      <codde>development</code>.
    </td>
  </tr>  
  <tr>
    <td>
      <code>endpoint</code>
    </td>
    <td>
      no
    </td>
    <td>
      Specify the enterprise Bugsnag endpoint. 
    </td>
  </tr>  
</table>


### newrelic

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>licensekey</code>
    </td>
    <td>
      yes
    </td>
    <td>
      License key provided by New Relic.
    </td>
  </tr>
   <tr>
    <td>
      <code>name</code>
    </td>
    <td>
      no
    </td>
    <td>
      New Relic application name.
    </td>
  </tr> 
     <tr>
    <td>
      <code>verbose</code>
    </td>
    <td>
      no
    </td>
    <td>
      Enable New Relic debugging output on stdout.
    </td>
  </tr> 
</table>

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

The `http` option details the configuration for the HTTP server that hosts the registry.

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>addr</code>
    </td>
    <td>
      yes
    </td>
    <td>
      The <code>HOST:PORT</code> for which the server should accept connections.
    </td>
  </tr>
    <tr>
    <td>
      <code>prefix</code>
    </td>
    <td>
      no
    </td>
    <td>
If the server does not run at the root path use this value to specify the
prefix. The root path is the section before <code>v2</code>. It
should have both preceding and trailing slashes, for example <code>/path/</code>.
    </td>
  </tr>
  <tr>
    <td>
      <code>secret</code>
    </td>
    <td>
      yes
    </td>
    <td>
A random piece of data. This is used to sign state that may be stored with the
client to protect against tampering. For production environments you should generate a
random piece of data using a cryptographically secure random generator.
    </td>
  </tr>
</table>


### tls

The `tls` struct within `http` is **optional**. Use this to configure TLS
for the server. If you already have a server such as Nginx or Apache running on
the same host as the registry, you may prefer to configure TLS termination there
and proxy connections to the registry server.

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>certificate</code>
    </td>
    <td>
      yes
    </td>
    <td>
       Absolute path to x509 cert file
    </td>
  </tr>
    <tr>
    <td>
      <code>key</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Absolute path to x509 private key file.
    </td>
  </tr>
  <tr>
    <td>
      <code>clientcas</code>
    </td>
    <td>
      no
    </td>
    <td>
      An array of absolute paths to a x509 CA file
    </td>
  </tr>  
</table>


### debug

The `debug` option is **optional** . Use it to configure a debug server that can
be helpful in diagnosing problems. Contributors to the distribution repository
should find the debug server useful. Docker recommends disabling it in
production environments.

The `debug` section takes a single, required `addr` parameter. This parameter
specifies the `HOST:PORT` on which the debug server should accept connections.


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

The notifications option is **optional** and currently may contain a single
option, `endpoints`.

### endpoints

Endpoints is a list of named services (URLs) that can accept event notifications.

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>name</code>
    </td>
    <td>
      yes
    </td>
    <td>
A human readable name for the service.     
</td>
  </tr>
  <tr>
    <td>
      <code>disabled</code>
    </td>
    <td>
      no
    </td>
    <td>
A boolean to enable/disable notifications for a service.
    </td>
  </tr>
  <tr>
    <td>
      <code>url</code>
    </td>
    <td>
		yes
    </td>
    <td>
The URL to which events should be published.
    </td>
  </tr>  
   <tr>
    <td>
      <code>headers</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Static headers to add to each request.
    </td>
  </tr> 
  <tr>
    <td>
      <code>timeout</code>
    </td>
    <td>
      yes
    </td>
    <td>
      An HTTP timeout value. This field takes a positive integer and an optional
      suffix indicating the unit of time. Possible units are:
      <ul>
      	<li><code>ns</code> (nanoseconds)</li>
      	<li><code>us</code> (microseconds)</li>
      	<li><code>ms</code> (milliseconds)</li>
      	<li><code>s</code> (seconds)</li>
      	<li><code>m</code> (minutes)</li>
        <li><code>h</code> (hours)</li>
      </ul>
    If you omit the suffix, the system interprets the value as nanoseconds.
    </td>
  </tr>  
  <tr>
    <td>
      <code>threshold</code>
    </td>
    <td>
      yes
    </td>
    <td>
      An integer specifying how long to wait before backing off a failure.
    </td>
  </tr>  
  <tr>
    <td>
      <code>backoff</code>
    </td>
    <td>
      yes
    </td>
    <td>
      How long the system backs off before retrying. This field takes a positive
      integer and an optional suffix indicating the unit of time. Possible units
      are:
      <ul>
      	<li><code>ns</code> (nanoseconds)</li>
      	<li><code>us</code> (microseconds)</li>
      	<li><code>ms</code> (milliseconds)</li>
      	<li><code>s</code> (seconds)</li>
      	<li><code>m</code> (minutes)</li>
        <li><code>h</code> (hours)</li>
      </ul>
    If you omit the suffix, the system interprets the value as nanoseconds.
    </td>
  </tr>  
</table>


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
may use the Redis instance for several applications. The current purpose is
caching information about immutable blobs. Most of the options below control
how the registry connects to redis. You can control the pool's behavior
with the [pool](#pool) subsection.

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>addr</code>
    </td>
    <td>
      yes
    </td>
    <td>
      Address (host and port) of redis instance.
    </td>
  </tr>
  <tr>
    <td>
      <code>password</code>
    </td>
    <td>
      no
    </td>
    <td>
      A password used to authenticate to the redis instance.
    </td>
  </tr>
  <tr>
    <td>
      <code>db</code>
    </td>
    <td>
      no
    </td>
    <td>
      Selects the db for each connection.
    </td>
  </tr>
  <tr>
    <td>
      <code>dialtimeout</code>
    </td>
    <td>
      no
    </td>
    <td>
      Timeout for connecting to a redis instance.
    </td>
  </tr>  
  <tr>
    <td>
      <code>readtimeout</code>
    </td>
    <td>
      no
    </td>
    <td>
      Timeout for reading from redis connections.
    </td>
  </tr>   
  <tr>
    <td>
      <code>writetimeout</code>
    </td>
    <td>
      no
    </td>
    <td>
      Timeout for writing to redis connections.
    </td>
  </tr>   
</table>


### pool

```yaml
pool:
	maxidle: 16
	maxactive: 64
	idletimeout: 300s
```

Configure the behavior of the Redis connection pool.

<table>
  <tr>
    <th>Parameter</th>
    <th>Required</th>
    <th>Description</th>
  </tr>
  <tr>
    <td>
      <code>maxidle</code>
    </td>
    <td>
      no
    </td>
    <td>
      Sets the maximum number of idle connections.
    </td>
  </tr>
  <tr>
    <td>
      <code>maxactive</code>
    </td>
    <td>
      no
    </td>
    <td>
      sets the maximum number of connections that should
  be opened before blocking a connection request.
    </td>
  </tr>
  <tr>
    <td>
      <code>idletimeout</code>
    </td>
    <td>
      no
    </td>
    <td>
      sets the amount time to wait before closing
  inactive connections.
    </td>
  </tr>  
</table>


## Example: Development configuration

The following is a simple example you can use for local development:

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

The above configures the registry instance to run on port `5000`, binding to
`localhost`, with the `debug` server enabled. Registry data storage is in the 
`/tmp/registry-dev` directory. Logging is in `debug` mode, which is the most
verbose.

A similar simple configuration is available at
[config.yml](https://github.com/docker/distribution/blob/master/cmd/registry/config.yml).
Both are generally useful for local development.


## Example: Middleware configuration

This example illustrates how to configure storage middleware in a registry.
Middleware allows the registry to serve layers via a content delivery network
(CDN). This is useful for reducing requests to the storage layer.  

Currently, the registry supports [Amazon
Cloudfront](http://aws.amazon.com/cloudfront/). You can only use Cloudfront in
conjunction with the S3 storage driver.

<table>
  <tr>
    <th>Parameter</th>
    <th>Description</th>
  </tr>
  <tr>
    <td><code>name</code></td>
    <td>The storage middleware name. Currently <code>cloudfront</code> is an accepted value.</td>
  </tr>
  <tr>
    <td><code>disabled<code></td>
    <td>Set to <code>false</code> to easily disable the middleware.</td>
  </tr>
  <tr>
    <td><code>options:</code></td>
    <td> 
    A set of key/value options to configure the middleware.
    <ul>
    <li><code>baseurl:</code> The Cloudfront base URL.</li>
    <li><code>privatekey:</code> The location of your AWS private key on the filesystem. </li>
    <li><code>keypairid:</code> The ID of your Cloudfront keypair. </li>
 		<li><code>duration:</code> The duration in minutes for which the URL is valid. Default is 20. </li>
 		</ul>
    </td>
  </tr>
</table>

The following example illustrates these values:

```
middleware:
    storage:
        - name: cloudfront
          disabled: false
          options:
             baseurl: http://d111111abcdef8.cloudfront.net
             privatekey: /path/to/asecret.pem
             keypairid: asecret
             duration: 60
```


>**Note**: Cloudfront keys exist separately to other AWS keys.  See
>[the documentation on AWS credentials](http://docs.aws.amazon.com/AWSSecurityCredentials/1.0/AboutAWSCredentials.html#KeyPairs)
>for more information.

