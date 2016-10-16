<!--[metadata]>
+++
title = "Oauth2 Token Authentication"
description = "Specifies the Docker Registry v2 authentication"
keywords = ["registry, on-prem, images, tags, repository, distribution, oauth2, advanced"]
[menu.main]
parent="smn_registry_ref"
weight=102
+++
<![end-metadata]-->

# Docker Registry v2 authentication using OAuth2

This document describes support for the OAuth2 protocol within the authorization
server. [RFC6749](https://tools.ietf.org/html/rfc6749) should be used as a
reference for the protocol and HTTP endpoints described here.

**Note**: Not all token servers implement oauth2. If the request to the endpoint
returns `404` using the HTTP `POST` method, refer to
[Token Documentation](token.md) for using the HTTP `GET` method supported by all
token servers.

## Refresh token format

The format of the refresh token is completely opaque to the client and should be
determined by the authorization server. The authorization should ensure the
token is sufficiently long and is responsible for storing any information about
long-lived tokens which may be needed for revoking. Any information stored
inside the token will not be extracted and presented by clients.

## Getting a token

POST /token

#### Headers
Content-Type: application/x-www-form-urlencoded

#### Post parameters

<dl>
    <dt>
        <code>grant_type</code>
    </dt>
    <dd>
        (REQUIRED) Type of grant used to get token. When getting a refresh token
        using credentials this type should be set to "password" and have the
        accompanying username and password paramters. Type "authorization_code"
        is used for authenticating to an authorization using a code given from
        directly from a resource owner and without having to send credentials
        through the client. When requesting an access token with a refresh token
        this should be set to "refresh_token".
    </dd>
    <dt>
        <code>service</code>
    </dt>
    <dd>
        (REQUIRED) The name of the service which hosts the resource to get
        access for. Refresh tokens will only be good for getting tokens for
        this service.
    </dd>
    <dt>
        <code>client_id</code>
    </dt>
    <dd>
        (REQUIRED) String identifying the client. This client_id does not need
        to be registered with the authorization server but should be set to a
        meaningful value in order to allow auditing keys created by unregistered
        clients. Accepted syntax is defined in
        [RFC6749 Appendix A.1](https://tools.ietf.org/html/rfc6749#appendix-A.1)
    </dd>
    <dt>
        <code>access_type</code>
    </dt>
    <dd>
        (OPTIONAL) Access which is being requested. If "offline" is provided
        then a refresh token will be returned. The default is "online" only
        returning short lived access token. If the grant type is "refresh_token"
        this will only return the same refresh token and not a new one.
    </dd>
    <dt>
        <code>scope</code>
    </dt>
    <dd>
        (OPTIONAL) The resource in question, formatted as one of the space-delimited
        entries from the <code>scope</code> parameters from the <code>WWW-Authenticate</code> header
        shown above. This query parameter should only be specified once but may
        contain multiple scopes using the scope list format defined in the scope
        grammar. If multiple <code>scope</code> is provided from
        <code>WWW-Authenticate</code> header the scopes should first be
        converted to a scope list before requesting the token. The above example
        would be specified as: <code>scope=repository:samalba/my-app:push</code>.
        When requesting a refresh token the scopes may be empty since the
        refresh token will not be limited by this scope, only the provided short
        lived access token will have the scope limitation.
    </dd>
    <dt>
        <code>refresh_token</code>
    </dt>
    <dd>
        (OPTIONAL) The refresh token to use for authentication when grant type "refresh_token" is used.
    </dd>
    <dt>
        <code>code</code>
    </dt>
    <dd>
        (OPTIONAL) The authorization to use for authentication when the grant
        type "authorization_code" is used.
    </dd>
    <dt>
        <code>username</code>
    </dt>
    <dd>
        (OPTIONAL) The username to use for authentication when grant type "password" is used.
    </dd>
    <dt>
        <code>password</code>
    </dt>
    <dd>
        (OPTIONAL) The password to use for authentication when grant type "password" is used.
    </dd>
</dl>

#### Response fields

<dl>
    <dt>
        <code>access_token</code>
    </dt>
    <dd>
        (REQUIRED) An opaque <code>Bearer</code> token that clients should
        supply to subsequent requests in the <code>Authorization</code> header.
        This token should not be attempted to be parsed or understood by the
        client but treated as opaque string.
    </dd>
    <dt>
        <code>scope</code>
    </dt>
    <dd>
        (REQUIRED) The scope granted inside the access token. This may be the
        same scope as requested or a subset. This requirement is stronger than
        specified in [RFC6749 Section 4.2.2](https://tools.ietf.org/html/rfc6749#section-4.2.2)
        by strictly requiring the scope in the return value.
    </dd>
    <dt>
        <code>expires_in</code>
    </dt>
    <dd>
        (REQUIRED) The duration in seconds since the token was issued that it
        will remain valid.  When omitted, this defaults to 60 seconds.  For
        compatibility with older clients, a token should never be returned with
        less than 60 seconds to live.
    </dd>
    <dt>
        <code>issued_at</code>
    </dt>
    <dd>
        (Optional) The <a href="https://www.ietf.org/rfc/rfc3339.txt">RFC3339</a>-serialized UTC
        standard time at which a given token was issued. If <code>issued_at</code> is omitted, the
        expiration is from when the token exchange completed.
    </dd>
    <dt>
        <code>refresh_token</code>
    </dt>
    <dd>
        (Optional) Token which can be used to get additional access tokens for
        the same subject with different scopes. This token should be kept secure
        by the client and only sent to the authorization server which issues
        bearer tokens. This field will only be set when `access_type=offline` is
        provided in the request.
    </dd>
</dl>

#### Example getting refresh token

```
POST /token HTTP/1.1
Host: auth.docker.io
Content-Type: application/x-www-form-urlencoded

grant_type=password&username=johndoe&password=A3ddj3w&service=hub.docker.io&client_id=dockerengine&access_type=offline

HTTP/1.1 200 OK
Content-Type: application/json

{"refresh_token":"kas9Da81Dfa8","access_token":"eyJhbGciOiJFUzI1NiIsInR5","expires_in":900,"scope":""}
```

#### Example refreshing an Access Token

```
POST /token HTTP/1.1
Host: auth.docker.io
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token=kas9Da81Dfa8&service=registry-1.docker.io&client_id=dockerengine&scope=repository:samalba/my-app:pull,push

HTTP/1.1 200 OK
Content-Type: application/json

{"refresh_token":"kas9Da81Dfa8","access_token":"eyJhbGciOiJFUzI1NiIsInR5":"expires_in":900,"scope":"repository:samalba/my-app:pull,repository:samalba/my-app:push"}
```

## Getting Authorization Challenge From Token Server

Before authenticating with the token server the client does a `HEAD`
request to the `/token` endpoint to get any authentication challenges.
If the token server supports OAuth2, it will return a `WWW-Authenticate`
with the type `OAuth2` and the parameters needed by the client to
complete the oauth2 flow. If the client has a username and password,
then the grant_type `password` will immediately be used. If no credentials
were provided the daemon will pass the challenge back to the Docker client
to complete the OAuth2 flow.

### Authentication Parameters

 - *client_id* - OAuth2 client id (cliend id used by provider not necessarily used by token server)
 - *auth_url* - Authorization endpoint to send client
 - *redirect_url* - Redirect location after login flow
 - *landing_url* - Location to redirect after login complete
 - *scopes* - Space separate list of scopes used by oauth provider, should be list of scopes needed registry and namespace identification.

#### Example Return Value

```
HEAD /token HTTP/1.1
Host: tokenserver.example.com

HTTP/1.1 401 Unauthorized
Www-Authenticate: OAuth2 client_id="93202393-o0asf811.apps.googleusercontent.com",auth_url="https://accounts.google.com/o/oauth2/auth",redirect_url="http://localhost:8082/oauth2callback",scopes="https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile",landing_url="https://docs.docker.com/registry/"
Date: Fri, 06 May 2016 05:40:23 GMT

```

### 3 Legged OAuth Flow

Client does not prompt the user for a username and password but instead
immediately hits the login endpoint on the daemon. The daemon will get
the challenge from the token server and return the challenge back
to the Docker client. The Docker client is responsible for completing
the oauth2 flow to get an authorization code and call login again
with the authorization code.

```
                   +------+        +------+
                   |Docker| Login  |Docker|
                   |Client| Without|Daemon|                           +--------+
                   |      | Creds  |      |                           | Docker |
                   |      +-------->      |    GET /v2/ (1)           |Registry|
                   |      |        |      +--------------------------->        |
                   |      |        |      |    401 Challenge          |        |
                   |      |        |      <---------------------------+        |
                   |      |        |      |                           |        |
                   |      |        |      |                 +------+  |        |
                   |      |        |      |  HEAD /token    |Token |  |        |
                   |      |        |      +----------------->Server|  |        |
+-------+          |      | OAuth2 |      |  401 Challenge  |      |  |        |
|  Web  | Redirect |      | Config |      <-----------------+      |  |        |
|Browser| Web login|      <--------+      |                 |      |  |        |
|       <----------+      | Login  |      |                 |      |  |        |
|       | Auth Code|      | With   |      |                 |      |  |        |
|       +---------->      | Code   |      |                 |      |  |        |
|       |          |      +-------->      |  POST /token    |      |  |        |
|       |          |      |        |      +----------------->      |  |        |
+-------+          |      |        |      |  Auth Tokens (2)|      |  |        |
                   |      |        |      <-----------------+      |  |        |
                   |      |        |      |                 |      |  |        |
                   |      |        |      |                 +------+  |        |
                   |      |        |      |  GET /v2/ (3)             |        |
                   |      |        |      +--------------------------->        |
                   |      | Login  |      |  OK                       |        |
                   |      | Success|      <---------------------------+        |
                   |      <--------+      |                           |        |
                   |      |        |      |                           +--------+
                   +------+        +------+

(1) No authorization headers should be included
(2) May return refresh token if offline access requested, always access token
(3) Access token from previous request used to validate login
```

### OAuth2 Flow With Credentials

The Docker client first prompts the user for credentials before
calling the login endpoint on the daemon. The credentials are
sent on login and when an oauth2 challenge is received from the
token server, the credentials are immediately used with the
`password` grant type.

```
+------+        +------+
|Docker| Login  |Docker|
|Client| With   |Daemon|                           +--------+
|      | Creds  |      |                           |Registry|
|      +-------->      |    GET /v2/               |        |
|      |        |      +--------------------------->        |
|      |        |      |    401 Challenge          |        |
|      |        |      <---------------------------+        |
|      |        |      |                           |        |
|      |        |      |                 +------+  |        |
|      |        |      |  HEAD /token    |      |  |        |
|      |        |      +----------------->Token |  |        |
|      |        |      |  401 Challenge  |Server|  |        |
|      |        |      <-----------------+      |  |        |
|      |        |      |                 |      |  |        |
|      |        |      |  POST /token    |      |  |        |
|      |        |      +----------------->      |  |        |
|      |        |      |  Refresh Token  |      |  |        |
|      |        |      <-----------------+      |  |        |
|      |        |      |                 |      |  |        |
|      |        |      |                 +------+  |        |
|      |        |      |  GET /v2/                 |        |
|      |        |      +--------------------------->        |
|      | Login  |      |  OK                       |        |
|      | Success|      <---------------------------+        |
|      <--------+      |                           |        |
|      |        |      |                           +--------+
+------+        +------+

```

## Compatibility With Older Token Servers

A token server which does not support OAuth2 should not
return a valid challenge and cause the client to fallback
to using the GET endpoint.

### OAuth2 Incompatible Token Server Flow

```
+------+         +------+
|Docker| Login   |Docker|
|Client| With    |Daemon|                           +--------+
|      | Password|      |                           |Registry|
|      +--------->      |    GET /v2/               |        |
|      |         |      +--------------------------->        |
|      |         |      |    401 Challenge          |        |
|      |         |      <---------------------------+        |
|      |         |      |                           |        |
|      |         |      |                 +------+  |        |
|      |         |      |  HEAD /token    |Token |  |        |
|      |         |      +----------------->Server|  |        |
|      |         |      |  No Challenge   |      |  |        |
|      |         |      <-----------------+      |  |        |
|      |         |      |                 |      |  |        |
|      |         |      |  GET /token     |      |  |        |
|      |         |      +----------------->      |  |        |
|      |         |      |  Auth Tokens    |      |  |        |
|      |         |      <-----------------+      |  |        |
|      |         |      |                 +------+  |        |
|      |         |      |                           |        |
|      |         |      |  GET /v2/                 |        |
|      |         |      +--------------------------->        |
|      | Login   |      |  200 OK                   |        |
|      | Success |      <---------------------------+        |
|      <---------+      |                           |        |
|      |         |      |                           +--------+
+------+         +------+

```
