<!--[metadata]>
+++
title = "Oauth2 Token Authentication"
description = "Specifies the Docker Registry v2 authentication"
keywords = ["registry, on-prem, images, tags, repository, distribution, oauth2, advanced"]
[menu.main]
parent="smn_registry_ref"
+++
<![end-metadata]-->

# Docker Registry v2 authentication using OAuth2

This document describes support for the OAuth2 protocol within the authorization
server. [RFC6749](https://tools.ietf.org/html/rfc6749) should be used as a
reference for the protocol and HTTP endpoints described here.

## Refresh token format

The format of the refresh token is completely opaque to the client and should be
determined by the authorization server. The authorization should ensure the
token is sufficiently long and is responsible for storing any information about
long-lived tokens which may be needed for revoking. Any information stored
inside the token will not be extracted and presented by clients.

## Getting a token

POST /token

#### Headers
Authorization headers
Content-Type: application/x-www-form-urlencoded

#### Post parameters

<dl>
    <dt>
        <code>grant_type</code>
    </dt>
    <dd>
        (REQUIRED) Type of grant used to get token. When getting a refresh token
        using credentials this type should be set to "password" and have the
        accompanying basic auth header. Type "authorization_code" is reserved
        for future use for authenticating to an authorization server without
        having to send credentials directly from the client. When requesting an
        access token with a refresh token this should be set to "refresh_token".
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
        <code>client</code>
    </dt>
    <dd>
        (REQUIRED) The name of the client which is getting accessed. Intended to be human
        readable for key auditing.
    </dd>
    <dt>
        <code>access_type</code>
    </dt>
    <dd>
        (OPTIONAL) Access which is being requested. If "offline" is provided then a refresh
        token will be returned. Otherwise only a short lived access token will
        be returned. If the grant type is "refresh_token" this will only return
        the same refresh token and not a new one.
    </dd>
    <dt>
        <code>scope</code>
    </dt>
    <dd>
        (OPTIONAL) The resource in question, formatted as one of the space-delimited
        entries from the <code>scope</code> parameters from the <code>WWW-Authenticate</code> header
        shown above. This query parameter should be specified multiple times if
        there is more than one <code>scope</code> entry from the <code>WWW-Authenticate</code>
        header. The above example would be specified as:
        <code>scope=repository:samalba/my-app:push</code>. When requesting a refresh
        token the scopes may be empty since the refresh token will not be limited by
        this scope, only the provided short lived access token.
    </dd>
    <dt>
        <code>refresh_token</code>
    </dt>
    <dd>
        (OPTIONAL) The refresh token to use for authentication when grant type "refresh_token" is used.
    </dd>
</dl>

#### Example getting refresh token

```
POST /token HTTP/1.1
Host: auth.docker.io
Authorization: ...
Content-Type: application/x-www-form-urlencoded

grant_type=password&service=hub.docker.io&client=dockerengine&access_type=offline

HTTP/1.1 200 OK
Content-Type: application/json

{"refresh_token":"xT2s5VFNrbzZTMVExUmpwWVRsSklPbFJMTmtnNlMxUkxOanBCUVV0VU1Ga3d","access_token":"eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDTHpDQ0FkU2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0Uk5Gb3pPa2RYTjBrNldGUlFSRHBJVFRSUk9rOVVWRmc2TmtGRlF6cFNUVE5ET2tGU01rTTZUMFkzTnpwQ1ZrVkJPa2xHUlVrNlExazFTekFlRncweE5UQTJNalV4T1RVMU5EWmFGdzB4TmpBMk1qUXhPVFUxTkRaYU1FWXhSREJDQmdOVkJBTVRPMGhHU1UwNldGZFZWam8yUVZkSU9sWlpUVEk2TTFnMVREcFNWREkxT2s5VFNrbzZTMVExUmpwWVRsSklPbFJMTmtnNlMxUkxOanBCUVV0VU1Ga3dFd1lIS29aSXpqMENBUVlJS29aSXpqMERBUWNEUWdBRXl2UzIvdEI3T3JlMkVxcGRDeFdtS1NqV1N2VmJ2TWUrWGVFTUNVMDByQjI0akNiUVhreFdmOSs0MUxQMlZNQ29BK0RMRkIwVjBGZGdwajlOWU5rL2pxT0JzakNCcnpBT0JnTlZIUThCQWY4RUJBTUNBSUF3RHdZRFZSMGxCQWd3QmdZRVZSMGxBREJFQmdOVkhRNEVQUVE3U0VaSlRUcFlWMVZXT2paQlYwZzZWbGxOTWpveldEVk1PbEpVTWpVNlQxTktTanBMVkRWR09saE9Va2c2VkVzMlNEcExWRXMyT2tGQlMxUXdSZ1lEVlIwakJEOHdQWUE3VVRSYU16cEhWemRKT2xoVVVFUTZTRTAwVVRwUFZGUllPalpCUlVNNlVrMHpRenBCVWpKRE9rOUdOemM2UWxaRlFUcEpSa1ZKT2tOWk5Vc3dDZ1lJS29aSXpqMEVBd0lEU1FBd1JnSWhBTXZiT2h4cHhrTktqSDRhMFBNS0lFdXRmTjZtRDFvMWs4ZEJOVGxuWVFudkFpRUF0YVJGSGJSR2o4ZlVSSzZ4UVJHRURvQm1ZZ3dZelR3Z3BMaGJBZzNOUmFvPSJdfQ.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImRtY2dvd2FuL2hlbGxvLXdvcmxkIiwiYWN0aW9ucyI6WyJwdWxsIl19XSwiYXVkIjoicmVnaXN0cnkuZG9ja2VyLmlvIiwiZXhwIjoxNDU0NDM4Njc1LCJpYXQiOjE0NTQ0MzgzNzUsImlzcyI6ImF1dGguZG9ja2VyLmlvIiwianRpIjoiZXFrVmVsWWJtbW5KSDctNW53SEkiLCJuYmYiOjE0NTQ0MzgzNzUsInN1YiI6ImRtY2dvd2FuIn0"}
````

#### Example refreshing an Access Token

````
POST /token HTTP/1.1
Host: auth.docker.io
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token=kas9Da81Dfa8&service=registry-1.docker.io&client=dockerengine&scope=repository:samalba/my-app:pull,push

HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDTHpDQ0FkU2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0Uk5Gb3pPa2RYTjBrNldGUlFSRHBJVFRSUk9rOVVWRmc2TmtGRlF6cFNUVE5ET2tGU01rTTZUMFkzTnpwQ1ZrVkJPa2xHUlVrNlExazFTekFlRncweE5UQTJNalV4T1RVMU5EWmFGdzB4TmpBMk1qUXhPVFUxTkRaYU1FWXhSREJDQmdOVkJBTVRPMGhHU1UwNldGZFZWam8yUVZkSU9sWlpUVEk2TTFnMVREcFNWREkxT2s5VFNrbzZTMVExUmpwWVRsSklPbFJMTmtnNlMxUkxOanBCUVV0VU1Ga3dFd1lIS29aSXpqMENBUVlJS29aSXpqMERBUWNEUWdBRXl2UzIvdEI3T3JlMkVxcGRDeFdtS1NqV1N2VmJ2TWUrWGVFTUNVMDByQjI0akNiUVhreFdmOSs0MUxQMlZNQ29BK0RMRkIwVjBGZGdwajlOWU5rL2pxT0JzakNCcnpBT0JnTlZIUThCQWY4RUJBTUNBSUF3RHdZRFZSMGxCQWd3QmdZRVZSMGxBREJFQmdOVkhRNEVQUVE3U0VaSlRUcFlWMVZXT2paQlYwZzZWbGxOTWpveldEVk1PbEpVTWpVNlQxTktTanBMVkRWR09saE9Va2c2VkVzMlNEcExWRXMyT2tGQlMxUXdSZ1lEVlIwakJEOHdQWUE3VVRSYU16cEhWemRKT2xoVVVFUTZTRTAwVVRwUFZGUllPalpCUlVNNlVrMHpRenBCVWpKRE9rOUdOemM2UWxaRlFUcEpSa1ZKT2tOWk5Vc3dDZ1lJS29aSXpqMEVBd0lEU1FBd1JnSWhBTXZiT2h4cHhrTktqSDRhMFBNS0lFdXRmTjZtRDFvMWs4ZEJOVGxuWVFudkFpRUF0YVJGSGJSR2o4ZlVSSzZ4UVJHRURvQm1ZZ3dZelR3Z3BMaGJBZzNOUmFvPSJdfQ.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImRtY2dvd2FuL2hlbGxvLXdvcmxkIiwiYWN0aW9ucyI6WyJwdWxsIl19XSwiYXVkIjoicmVnaXN0cnkuZG9ja2VyLmlvIiwiZXhwIjoxNDU0NDM4Njc1LCJpYXQiOjE0NTQ0MzgzNzUsImlzcyI6ImF1dGguZG9ja2VyLmlvIiwianRpIjoiZXFrVmVsWWJtbW5KSDctNW53SEkiLCJuYmYiOjE0NTQ0MzgzNzUsInN1YiI6ImRtY2dvd2FuIn0"}
````

