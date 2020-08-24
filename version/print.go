package version

import (
	"fmt"
	"io"
	"os"
)

// FprintVersion outputs the version string to the writer, in the following
// format, followed by a newline:
//
// 	<cmd> <project> <version>
//
// For example, a binary "registry" built from github.com/distribution/distribution
// with version "v2.0" would print the following:
//
// 	registry github.com/distribution/distribution v2.0
//
func FprintVersion(w io.Writer) {
	fmt.Fprintln(w, os.Args[0], Package, Version)
}

// PrintVersion outputs the version information, from Fprint, to stdout.
func PrintVersion() {
	FprintVersion(os.Stdout)
}
