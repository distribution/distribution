package basic

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"os"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ErrAuthenticationFailure A generic error message for authentication failure to be presented to agent.
var ErrAuthenticationFailure = errors.New("Bad username or password")

// htpasswd Holds a path to a system .htpasswd file and the machinery to parse it.
type htpasswd struct {
	path   string
	reader *csv.Reader
}

// AuthType Represents a particular hash function used in the htpasswd file.
type AuthType int

const (
	// PlainText Plain-text password storage (htpasswd -p)
	PlainText AuthType = iota
	// SHA1 sha hashed password storage (htpasswd -s)
	SHA1
	// ApacheMD5 apr iterated md5 hashing (htpasswd -m)
	ApacheMD5
	// BCrypt BCrypt adapative password hashing (htpasswd -B)
	BCrypt
	// Crypt System crypt() hashes.  (htpasswd -d)
	Crypt
)

// String Returns a text representation of the AuthType
func (at AuthType) String() string {
	switch at {
	case PlainText:
		return "plaintext"
	case SHA1:
		return "sha1"
	case ApacheMD5:
		return "md5"
	case BCrypt:
		return "bcrypt"
	case Crypt:
		return "system crypt"
	}
	return "unknown"
}

// NewHTPasswd Create a new HTPasswd with the given path to .htpasswd file.
func NewHTPasswd(htpath string) *htpasswd {
	return &htpasswd{path: htpath}
}

var bcryptPrefixRegexp = regexp.MustCompile(`^\$2[ab]?y\$`)

// GetAuthCredentialType Inspect an htpasswd file credential and guess the encryption algorithm used.
func GetAuthCredentialType(cred string) AuthType {
	if strings.HasPrefix(cred, "{SHA}") {
		return SHA1
	}
	if strings.HasPrefix(cred, "$apr1$") {
		return ApacheMD5
	}
	if bcryptPrefixRegexp.MatchString(cred) {
		return BCrypt
	}
	// There's just not a great way to distinguish between these next two...
	if len(cred) == 13 {
		return Crypt
	}
	return PlainText
}

// AuthenticateUser Check a given user:password credential against the receiving HTPasswd's file.
func (htpasswd *htpasswd) AuthenticateUser(user string, pwd string) (bool, error) {

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
		if readerr != nil {
			return false, readerr
		}
		if len(entry) == 0 {
			continue
		}
		if entry[0] == user {
			credential := entry[1]
			credType := GetAuthCredentialType(credential)
			switch credType {
			case SHA1:
				{
					sha := sha1.New()
					sha.Write([]byte(pwd))
					hash := base64.StdEncoding.EncodeToString(sha.Sum(nil))
					return entry[1][5:] == hash, nil
				}
			case ApacheMD5:
				{
					return false, errors.New(ApacheMD5.String() + " htpasswd hash function not yet supported")
				}
			case BCrypt:
				{
					err := bcrypt.CompareHashAndPassword([]byte(credential), []byte(pwd))
					if err != nil {
						return false, err
					}
					return true, nil
				}
			case Crypt:
				{
					return false, errors.New(Crypt.String() + " htpasswd hash function not yet supported")
				}
			case PlainText:
				{
					if pwd == credential {
						return true, nil
					}
					return false, ErrAuthenticationFailure
				}
			}
		}
	}
	return false, ErrAuthenticationFailure
}
