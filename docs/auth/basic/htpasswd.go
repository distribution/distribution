package basic

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"os"
)

var ErrSHARequired = errors.New("htpasswd file must use SHA (htpasswd -s)")

type HTPasswd struct {
	path   string
	reader *csv.Reader
}

func NewHTPasswd(htpath string) *HTPasswd {
	return &HTPasswd{path: htpath}
}

func (htpasswd *HTPasswd) AuthenticateUser(user string, pwd string) (bool, error) {

	// Hash the credential.
	sha := sha1.New()
	sha.Write([]byte(pwd))
	hash := base64.StdEncoding.EncodeToString(sha.Sum(nil))

	// Open the file.
	in, err := os.Open(htpasswd.path)
	if err != nil {
		return false, err
	}

	// Parse the contents of the standard .htpasswd until we hit the end or find a match.
	reader := csv.NewReader(in)
	reader.Comma = ':'
	reader.Comment = '#'
	reader.TrimLeadingSpace = true
	for entry, readerr := reader.Read(); entry != nil || readerr != nil; entry, readerr = reader.Read() {
		if entry[0] == user {
			if len(entry[1]) < 6 || entry[1][0:5] != "{SHA}" {
				return false, ErrSHARequired
			}
			return entry[1][5:] == hash, nil
		}
	}
	return false, nil
}
