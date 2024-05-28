package version

import (
	"fmt"
	"io"
	"os"
)

// Package returns the overall, canonical project import path under
// which the package was built.
func Package() string {
	return mainpkg
}

// Version returns returns the module version the running binary was
// built from.
func Version() string {
	return version
}

// Revision returns the VCS (e.g. git) revision being used to build
// the program at linking time.
func Revision() string {
	return revision
}

// FprintVersion outputs the version string to the writer, in the following
// format, followed by a newline:
//
//	<cmd> <project> <version>
//
// For example, a binary "registry" built from github.com/distribution/distribution
// with version "v2.0" would print the following:
//
//	registry github.com/distribution/distribution v2.0
func FprintVersion(w io.Writer) {
	fmt.Fprintln(w, os.Args[0], Package(), Version())
}

// PrintVersion outputs the version information, from Fprint, to stdout.
func PrintVersion() {
	FprintVersion(os.Stdout)
}
