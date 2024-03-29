#!/bin/sh

# This bash script outputs the current, desired content of version.go, using
# git describe. For best effect, pipe this to the target file. Generally, this
# only needs to updated for releases. The actual value of will be replaced
# during build time if the makefile is used.

set -e

cat <<EOF
package version

// mainpkg is the overall, canonical project import path under which the
// package was built.
var mainpkg = "$(go list -m)"

// version indicates which version of the binary is running. This is set to
// the latest release tag by hand, always suffixed by "+unknown". During
// build, it will be replaced by the actual version. The value here will be
// used if the registry is run after a go get based install.
var version = "$(git describe --match 'v[0-9]*' --dirty='.m' --always)+unknown"

// revision is filled with the VCS (e.g. git) revision being used to build
// the program at linking time.
var revision = ""
EOF
