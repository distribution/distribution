// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import "net/url"

// safeURL removes sensitive information from a URL.
func safeURL(u *url.URL) string {
	if nil == u {
		return ""
	}
	if "" != u.Opaque {
		// If the URL is opaque, we cannot be sure if it contains
		// sensitive information.
		return ""
	}

	// Omit user, query, and fragment information for security.
	ur := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}
	return ur.String()
}

// safeURLFromString removes sensitive information from a URL.
func safeURLFromString(rawurl string) string {
	u, err := url.Parse(rawurl)
	if nil != err {
		return ""
	}
	return safeURL(u)
}

// hostFromURL returns the URL's host.
func hostFromURL(u *url.URL) string {
	if nil == u {
		return ""
	}
	if "" != u.Opaque {
		return "opaque"
	}
	return u.Host
}
