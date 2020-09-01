// +build go1.8

package panicwrap

import "os"

func Executable() (string, error) {
	return os.Executable()
}
