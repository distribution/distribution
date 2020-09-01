// +build !go1.8

package panicwrap

import "github.com/kardianos/osext"

func Executable() (string, error) {
	return osext.Executable()
}
