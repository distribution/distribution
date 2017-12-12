// Copyright 2017 The oauth2 Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package azure provides constants for using OAuth2 to access Azure Active Directory (Azure AD).
package azure

import (
	"golang.org/x/oauth2"
)

// Endpoint is the Azure Active Directory (Azure AD) OAuth 2.0 endpoint.
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
}
