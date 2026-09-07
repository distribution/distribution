package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// HashAlgorithm determines the hash algorithm used when signing CloudFront
// URLs and cookies. The zero value (empty string) defaults to SHA-1 for
// backward compatibility.
type HashAlgorithm string

const (
	// HashSHA1 uses SHA-1 for signing (default, backward compatible).
	HashSHA1 HashAlgorithm = "SHA1"

	// HashSHA256 uses SHA-256 for signing. Requires appending
	// Hash-Algorithm=SHA256 to the signed URL or cookie.
	HashSHA256 HashAlgorithm = "SHA256"
)

func (h HashAlgorithm) hash() (hash.Hash, crypto.Hash, error) {
	switch h {
	case HashSHA256:
		return sha256.New(), crypto.SHA256, nil
	case HashSHA1, "":
		return sha1.New(), crypto.SHA1, nil
	default:
		return nil, 0, fmt.Errorf("unsupported hash algorithm %q", h)
	}
}

// An AWSEpochTime wraps a time value providing JSON serialization needed for
// AWS Policy epoch time fields.
type AWSEpochTime struct {
	time.Time
}

// NewAWSEpochTime returns a new AWSEpochTime pointer wrapping the Go time provided.
func NewAWSEpochTime(t time.Time) *AWSEpochTime {
	return &AWSEpochTime{t}
}

// MarshalJSON serializes the epoch time as AWS Profile epoch time.
func (t AWSEpochTime) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `{"AWS:EpochTime":%d}`, t.UTC().Unix()), nil
}

// UnmarshalJSON unserializes AWS Profile epoch time.
func (t *AWSEpochTime) UnmarshalJSON(data []byte) error {
	var epochTime struct {
		Sec int64 `json:"AWS:EpochTime"`
	}
	err := json.Unmarshal(data, &epochTime)
	if err != nil {
		return err
	}
	t.Time = time.Unix(epochTime.Sec, 0).UTC()
	return nil
}

// An IPAddress wraps an IPAddress source IP providing JSON serialization information
type IPAddress struct {
	SourceIP string `json:"AWS:SourceIp"`
}

// A Condition defines the restrictions for how a signed URL can be used.
type Condition struct {
	// Optional IP address mask the signed URL must be requested from.
	IPAddress *IPAddress `json:"IpAddress,omitempty"`

	// Optional date that the signed URL cannot be used until. It is invalid
	// to make requests with the signed URL prior to this date.
	DateGreaterThan *AWSEpochTime `json:",omitempty"`

	// Required date that the signed URL will expire. A DateLessThan is required
	// sign cloud front URLs
	DateLessThan *AWSEpochTime `json:",omitempty"`
}

// A Statement is a collection of conditions for resources
type Statement struct {
	// The Web or RTMP resource the URL will be signed for
	Resource string

	// The set of conditions for this resource
	Condition Condition
}

// A Policy defines the resources that a signed will be signed for.
//
// See the following page for more information on how policies are constructed.
// http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-creating-signed-url-custom-policy.html#private-content-custom-policy-statement
type Policy struct {
	// List of resource and condition statements.
	// Signed URLs should only provide a single statement.
	Statements []Statement `json:"Statement"`
}

// Override for testing to mock out usage of crypto/rand.Reader
var randReader = rand.Reader

// Sign will sign a policy using an RSA private key. It will return a base 64
// encoded signature and policy if no error is encountered.
//
// The signature and policy should be added to the signed URL following the
// guidelines in:
// http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-signed-urls.html
func (p *Policy) Sign(signer crypto.Signer) (b64Signature, b64Policy []byte, err error) {
	return p.SignWithAlgorithm(signer, HashSHA1)
}

// SignWithAlgorithm will sign a policy using the specified hash algorithm. It will
// return a base 64 encoded signature and policy if no error is encountered.
func (p *Policy) SignWithAlgorithm(signer crypto.Signer, hash HashAlgorithm) (b64Signature, b64Policy []byte, err error) {
	if err = p.Validate(); err != nil {
		return nil, nil, err
	}

	b64Policy, jsonPolicy, err := encodePolicy(p)
	if err != nil {
		return nil, nil, err
	}
	awsEscapeEncoded(b64Policy)

	b64Signature, err = signEncodedPolicy(randReader, jsonPolicy, signer, hash)
	if err != nil {
		return nil, nil, err
	}
	awsEscapeEncoded(b64Signature)

	return b64Signature, b64Policy, nil
}

// Validate verifies that the policy is valid and usable, and returns an
// error if there is a problem.
func (p *Policy) Validate() error {
	if len(p.Statements) == 0 {
		return fmt.Errorf("at least one policy statement is required")
	}
	for i, s := range p.Statements {
		if s.Resource == "" {
			return fmt.Errorf("statement at index %d does not have a resource", i)
		}
		if !isASCII(s.Resource) {
			return fmt.Errorf("unable to sign resource, [%s]. "+
				"Resources must only contain ascii characters. "+
				"Hostnames with unicode should be encoded as Punycode, (e.g. golang.org/x/net/idna), "+
				"and URL unicode path/query characters should be escaped.", s.Resource)
		}
	}

	return nil
}

// CreateResource constructs, validates, and returns a resource URL string. An
// error will be returned if unable to create the resource string.
func CreateResource(scheme, u string) (string, error) {
	scheme = strings.ToLower(scheme)

	if scheme == "http" || scheme == "https" || scheme == "http*" || scheme == "*" {
		return u, nil
	}

	if scheme == "rtmp" {
		parsed, err := url.Parse(u)
		if err != nil {
			return "", fmt.Errorf("unable to parse rtmp URL, err: %s", err)
		}

		rtmpURL := strings.TrimLeft(parsed.Path, "/")
		if parsed.RawQuery != "" {
			rtmpURL = fmt.Sprintf("%s?%s", rtmpURL, parsed.RawQuery)
		}

		return rtmpURL, nil
	}

	return "", fmt.Errorf("invalid URL scheme must be http, https, or rtmp. Provided: %s", scheme)
}

// NewCannedPolicy returns a new Canned Policy constructed using the resource
// and expires time. This can be used to generate the basic model for a Policy
// that can be then augmented with additional conditions.
//
// See the following page for more information on how policies are constructed.
// http://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-creating-signed-url-custom-policy.html#private-content-custom-policy-statement
func NewCannedPolicy(resource string, expires time.Time) *Policy {
	return &Policy{
		Statements: []Statement{
			{
				Resource: resource,
				Condition: Condition{
					DateLessThan: NewAWSEpochTime(expires),
				},
			},
		},
	}
}

// encodePolicy encodes the Policy as JSON and also base 64 encodes it.
func encodePolicy(p *Policy) (b64Policy, jsonPolicy []byte, err error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(p); err != nil {
		return nil, nil, fmt.Errorf("failed to encode policy, %s", err.Error())
	}
	jsonPolicy = buffer.Bytes()
	// Remove leading and trailing white space, JSON encoding will note include
	// whitespace within the encoding.
	jsonPolicy = bytes.TrimSpace(jsonPolicy)

	b64Policy = make([]byte, base64.StdEncoding.EncodedLen(len(jsonPolicy)))
	base64.StdEncoding.Encode(b64Policy, jsonPolicy)
	return b64Policy, jsonPolicy, nil
}

// signEncodedPolicy will sign and base 64 encode the JSON encoded policy.
func signEncodedPolicy(randReader io.Reader, jsonPolicy []byte, signer crypto.Signer, hashAlg HashAlgorithm) ([]byte, error) {
	h, cryptoHash, err := hashAlg.hash()
	if err != nil {
		return nil, err
	}
	if _, err := bytes.NewReader(jsonPolicy).WriteTo(h); err != nil {
		return nil, fmt.Errorf("failed to calculate signing hash, %s", err.Error())
	}

	sig, err := signer.Sign(randReader, h.Sum(nil), cryptoHash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign policy, %s", err.Error())
	}

	b64Sig := make([]byte, base64.StdEncoding.EncodedLen(len(sig)))
	base64.StdEncoding.Encode(b64Sig, sig)
	return b64Sig, nil
}

// special characters to be replaced with awsEscapeEncoded
var invalidEncodedChar = map[byte]byte{
	'+': '-',
	'=': '_',
	'/': '~',
}

// awsEscapeEncoded will replace base64 encoding's special characters to be URL safe.
func awsEscapeEncoded(b []byte) {
	for i, v := range b {
		if r, ok := invalidEncodedChar[v]; ok {
			b[i] = r
		}
	}
}

func isASCII(u string) bool {
	for _, c := range u {
		if c > unicode.MaxASCII {
			return false
		}
	}
	return true
}
