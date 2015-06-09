package basic

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/docker/distribution/context"
	"golang.org/x/crypto/bcrypt"
)

// htpasswd holds a path to a system .htpasswd file and the machinery to parse it.
type htpasswd struct {
	path string
}

// authType represents a particular hash function used in the htpasswd file.
type authType int

const (
	authTypePlainText authType = iota // Plain-text password storage (htpasswd -p)
	authTypeSHA1                      // sha hashed password storage (htpasswd -s)
	authTypeApacheMD5                 // apr iterated md5 hashing (htpasswd -m)
	authTypeBCrypt                    // BCrypt adapative password hashing (htpasswd -B)
	authTypeCrypt                     // System crypt() hashes.  (htpasswd -d)
)

var bcryptPrefixRegexp = regexp.MustCompile(`^\$2[ab]?y\$`)

// detectAuthCredentialType inspects the credential and resolves the encryption scheme.
func detectAuthCredentialType(cred string) authType {
	if strings.HasPrefix(cred, "{SHA}") {
		return authTypeSHA1
	}
	if strings.HasPrefix(cred, "$apr1$") {
		return authTypeApacheMD5
	}
	if bcryptPrefixRegexp.MatchString(cred) {
		return authTypeBCrypt
	}
	// There's just not a great way to distinguish between these next two...
	if len(cred) == 13 {
		return authTypeCrypt
	}
	return authTypePlainText
}

// String Returns a text representation of the AuthType
func (at authType) String() string {
	switch at {
	case authTypePlainText:
		return "plaintext"
	case authTypeSHA1:
		return "sha1"
	case authTypeApacheMD5:
		return "md5"
	case authTypeBCrypt:
		return "bcrypt"
	case authTypeCrypt:
		return "system crypt"
	}
	return "unknown"
}

// NewHTPasswd Create a new HTPasswd with the given path to .htpasswd file.
func newHTPasswd(htpath string) *htpasswd {
	return &htpasswd{path: htpath}
}

// AuthenticateUser checks a given user:password credential against the
// receiving HTPasswd's file. If the check passes, nil is returned. Note that
// this parses the htpasswd file on each request so ensure that updates are
// available.
func (htpasswd *htpasswd) authenticateUser(ctx context.Context, username string, password string) error {
	// Open the file.
	in, err := os.Open(htpasswd.path)
	if err != nil {
		return err
	}
	defer in.Close()

	for _, entry := range parseHTPasswd(ctx, in) {
		if entry.username != username {
			continue // wrong entry
		}

		switch t := detectAuthCredentialType(entry.password); t {
		case authTypeSHA1:
			sha := sha1.New()
			sha.Write([]byte(password))
			hash := base64.StdEncoding.EncodeToString(sha.Sum(nil))

			if entry.password[5:] != hash {
				return ErrAuthenticationFailure
			}

			return nil
		case authTypeBCrypt:
			err := bcrypt.CompareHashAndPassword([]byte(entry.password), []byte(password))
			if err != nil {
				return ErrAuthenticationFailure
			}

			return nil
		case authTypePlainText:
			if password != entry.password {
				return ErrAuthenticationFailure
			}

			return nil
		default:
			context.GetLogger(ctx).Errorf("unsupported basic authentication type: %v", t)
		}
	}

	return ErrAuthenticationFailure
}

// htpasswdEntry represents a line in an htpasswd file.
type htpasswdEntry struct {
	username string // username, plain text
	password string // stores hashed passwd
}

// parseHTPasswd parses the contents of htpasswd. Bad entries are skipped and
// logged, so this may return empty. This will read all the entries in the
// file, whether or not they are needed.
func parseHTPasswd(ctx context.Context, rd io.Reader) []htpasswdEntry {
	entries := []htpasswdEntry{}
	scanner := bufio.NewScanner(rd)
	for scanner.Scan() {
		t := strings.TrimSpace(scanner.Text())
		i := strings.Index(t, ":")
		if i < 0 || i >= len(t) {
			context.GetLogger(ctx).Errorf("bad entry in htpasswd: %q", t)
			continue
		}

		entries = append(entries, htpasswdEntry{
			username: t[:i],
			password: t[i+1:],
		})
	}

	return entries
}
