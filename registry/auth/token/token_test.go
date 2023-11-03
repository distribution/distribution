package token

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
)

func makeRootKeys(numKeys int) ([]*ecdsa.PrivateKey, error) {
	rootKeys := make([]*ecdsa.PrivateKey, 0, numKeys)

	for i := 0; i < numKeys; i++ {
		pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		rootKeys = append(rootKeys, pk)
	}

	return rootKeys, nil
}

func makeRootCerts(rootKeys []*ecdsa.PrivateKey) ([]*x509.Certificate, error) {
	rootCerts := make([]*x509.Certificate, 0, len(rootKeys))

	for _, rootKey := range rootKeys {
		cert, err := generateCACert(rootKey, rootKey)
		if err != nil {
			return nil, err
		}
		rootCerts = append(rootCerts, cert)
	}

	return rootCerts, nil
}

func makeSigningKeyWithChain(rootKey *ecdsa.PrivateKey, depth int) (*jose.JSONWebKey, error) {
	if depth == 0 {
		// Don't need to build a chain.
		return &jose.JSONWebKey{
			Key:       rootKey,
			KeyID:     rootKey.X.String(),
			Algorithm: string(jose.ES256),
		}, nil
	}

	var (
		certs     = make([]*x509.Certificate, depth)
		parentKey = rootKey

		pk   *ecdsa.PrivateKey
		cert *x509.Certificate
		err  error
	)

	for depth > 0 {
		if pk, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return nil, err
		}

		if cert, err = generateCACert(parentKey, pk); err != nil {
			return nil, err
		}

		depth--
		certs[depth] = cert
		parentKey = pk
	}

	return &jose.JSONWebKey{
		Key:          parentKey,
		KeyID:        rootKey.X.String(),
		Algorithm:    string(jose.ES256),
		Certificates: certs,
	}, nil
}

func makeTestToken(jwk *jose.JSONWebKey, issuer, audience string, access []*ResourceActions, now time.Time, exp time.Time) (*Token, error) {
	signingKey := jose.SigningKey{
		Algorithm: jose.ES256,
		Key:       jwk,
	}
	signerOpts := jose.SignerOptions{
		EmbedJWK: true,
	}
	signerOpts.WithType("JWT")

	signer, err := jose.NewSigner(signingKey, &signerOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to create a signer: %s", err)
	}

	randomBytes := make([]byte, 15)
	if _, err = rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("unable to read random bytes for jwt id: %s", err)
	}

	claimSet := &ClaimSet{
		Issuer:     issuer,
		Subject:    "foo",
		Audience:   []string{audience},
		Expiration: exp.Unix(),
		NotBefore:  now.Unix(),
		IssuedAt:   now.Unix(),
		JWTID:      base64.URLEncoding.EncodeToString(randomBytes),
		Access:     access,
	}

	tokenString, err := jwt.Signed(signer).Claims(claimSet).CompactSerialize()
	if err != nil {
		return nil, fmt.Errorf("unable to build token string: %v", err)
	}

	return NewToken(tokenString)
}

// NOTE(milosgajdos): certTemplateInfo type as well
// as some of the functions in this file have been
// adopted from https://github.com/docker/libtrust
// and modiified to fit the purpose of the token package.

type certTemplateInfo struct {
	commonName  string
	domains     []string
	ipAddresses []net.IP
	isCA        bool
	clientAuth  bool
	serverAuth  bool
}

func generateCertTemplate(info *certTemplateInfo) *x509.Certificate {
	// Generate a certificate template which is valid from the past week to
	// 10 years from now. The usage of the certificate depends on the
	// specified fields in the given certTempInfo object.
	var (
		keyUsage    x509.KeyUsage
		extKeyUsage []x509.ExtKeyUsage
	)

	if info.isCA {
		keyUsage = x509.KeyUsageCertSign
	}

	if info.clientAuth {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageClientAuth)
	}

	if info.serverAuth {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	return &x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName: info.commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour * 24 * 7),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365 * 10),
		DNSNames:              info.domains,
		IPAddresses:           info.ipAddresses,
		IsCA:                  info.isCA,
		KeyUsage:              keyUsage,
		ExtKeyUsage:           extKeyUsage,
		BasicConstraintsValid: info.isCA,
	}
}

func generateCert(priv crypto.PrivateKey, pub crypto.PublicKey, subInfo, issInfo *certTemplateInfo) (*x509.Certificate, error) {
	pubCertTemplate := generateCertTemplate(subInfo)
	privCertTemplate := generateCertTemplate(issInfo)

	certDER, err := x509.CreateCertificate(
		rand.Reader, pubCertTemplate, privCertTemplate,
		pub, priv,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %s", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %s", err)
	}

	return cert, nil
}

// generateCACert creates a certificate which can be used as a trusted
// certificate authority.
func generateCACert(signer *ecdsa.PrivateKey, trustedKey *ecdsa.PrivateKey) (*x509.Certificate, error) {
	subjectInfo := &certTemplateInfo{
		commonName: trustedKey.X.String(),
		isCA:       true,
	}
	issuerInfo := &certTemplateInfo{
		commonName: signer.X.String(),
	}

	return generateCert(signer, trustedKey.Public(), subjectInfo, issuerInfo)
}

// This test makes 4 tokens with a varying number of intermediate
// certificates ranging from no intermediate chain to a length of 3
// intermediates.
func TestTokenVerify(t *testing.T) {
	var (
		numTokens = 4
		issuer    = "test-issuer"
		audience  = "test-audience"
		access    = []*ResourceActions{
			{
				Type:    "repository",
				Name:    "foo/bar",
				Actions: []string{"pull", "push"},
			},
		}
	)

	rootKeys, err := makeRootKeys(numTokens)
	if err != nil {
		t.Fatal(err)
	}

	rootCerts, err := makeRootCerts(rootKeys)
	if err != nil {
		t.Fatal(err)
	}

	rootPool := x509.NewCertPool()
	for _, rootCert := range rootCerts {
		rootPool.AddCert(rootCert)
	}

	tokens := make([]*Token, 0, numTokens)
	trustedKeys := map[string]crypto.PublicKey{}

	for i := 0; i < numTokens; i++ {
		jwk, err := makeSigningKeyWithChain(rootKeys[i], i)
		if err != nil {
			t.Fatal(err)
		}
		// add to trusted keys
		trustedKeys[jwk.KeyID] = jwk.Public()
		token, err := makeTestToken(jwk, issuer, audience, access, time.Now(), time.Now().Add(5*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}

	verifyOps := VerifyOptions{
		TrustedIssuers:    []string{issuer},
		AcceptedAudiences: []string{audience},
		Roots:             rootPool,
		TrustedKeys:       trustedKeys,
	}

	for _, token := range tokens {
		if _, err := token.Verify(verifyOps); err != nil {
			t.Fatal(err)
		}
	}
}

// This tests that we don't fail tokens with nbf within
// the defined leeway in seconds
func TestLeeway(t *testing.T) {
	var (
		issuer   = "test-issuer"
		audience = "test-audience"
		access   = []*ResourceActions{
			{
				Type:    "repository",
				Name:    "foo/bar",
				Actions: []string{"pull", "push"},
			},
		}
	)

	rootKeys, err := makeRootKeys(1)
	if err != nil {
		t.Fatal(err)
	}

	jwk, err := makeSigningKeyWithChain(rootKeys[0], 0)
	if err != nil {
		t.Fatal(err)
	}

	trustedKeys := map[string]crypto.PublicKey{
		jwk.KeyID: jwk.Public(),
	}

	verifyOps := VerifyOptions{
		TrustedIssuers:    []string{issuer},
		AcceptedAudiences: []string{audience},
		Roots:             nil,
		TrustedKeys:       trustedKeys,
	}

	// nbf verification should pass within leeway
	futureNow := time.Now().Add(time.Duration(5) * time.Second)
	token, err := makeTestToken(jwk, issuer, audience, access, futureNow, futureNow.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := token.Verify(verifyOps); err != nil {
		t.Fatal(err)
	}

	// nbf verification should fail with a skew larger than leeway
	futureNow = time.Now().Add(time.Duration(61) * time.Second)
	token, err = makeTestToken(jwk, issuer, audience, access, futureNow, futureNow.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = token.Verify(verifyOps); err == nil {
		t.Fatal("Verification should fail for token with nbf in the future outside leeway")
	}

	// exp verification should pass within leeway
	token, err = makeTestToken(jwk, issuer, audience, access, time.Now(), time.Now().Add(-59*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = token.Verify(verifyOps); err != nil {
		t.Fatal(err)
	}

	// exp verification should fail with a skew larger than leeway
	token, err = makeTestToken(jwk, issuer, audience, access, time.Now(), time.Now().Add(-60*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if _, err = token.Verify(verifyOps); err == nil {
		t.Fatal("Verification should fail for token with exp in the future outside leeway")
	}
}

func writeTempRootCerts(rootKeys []*ecdsa.PrivateKey) (filename string, err error) {
	rootCerts, err := makeRootCerts(rootKeys)
	if err != nil {
		return "", err
	}

	tempFile, err := os.CreateTemp("", "rootCertBundle")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	for _, cert := range rootCerts {
		if err = pem.Encode(tempFile, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		}); err != nil {
			os.Remove(tempFile.Name())
			return "", err
		}
	}

	return tempFile.Name(), nil
}

func writeTempJWKS(rootKeys []*ecdsa.PrivateKey) (filename string, err error) {
	keys := make([]jose.JSONWebKey, len(rootKeys))
	for i := range rootKeys {
		jwk, err := makeSigningKeyWithChain(rootKeys[i], i)
		if err != nil {
			return "", err
		}
		keys[i] = *jwk
	}
	jwks := jose.JSONWebKeySet{
		Keys: keys,
	}
	tempFile, err := os.CreateTemp("", "jwksBundle")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	if err := json.NewEncoder(tempFile).Encode(jwks); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

// TestAccessController tests complete integration of the token auth package.
// It starts by mocking the options for a token auth accessController which
// it creates. It then tries a few mock requests:
//   - don't supply a token; should error with challenge
//   - supply an invalid token; should error with challenge
//   - supply a token with insufficient access; should error with challenge
//   - supply a valid token; should not error
func TestAccessController(t *testing.T) {
	// Make 2 keys; only the first is to be a trusted root key.
	rootKeys, err := makeRootKeys(2)
	if err != nil {
		t.Fatal(err)
	}

	rootCertBundleFilename, err := writeTempRootCerts(rootKeys[:1])
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(rootCertBundleFilename)

	jwksFilename, err := writeTempJWKS(rootKeys)
	if err != nil {
		t.Fatal(err)
	}

	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]interface{}{
		"realm":          realm,
		"issuer":         issuer,
		"service":        service,
		"rootcertbundle": rootCertBundleFilename,
		"autoredirect":   false,
		"jwks":           jwksFilename,
	}

	accessController, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Make a mock http.Request with no token.
	req, err := http.NewRequest(http.MethodGet, "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	testAccess := auth.Access{
		Resource: auth.Resource{
			Type: "foo",
			Name: "bar",
		},
		Action: "baz",
	}

	grant, err := accessController.Authorized(req, testAccess)
	challenge, ok := err.(auth.Challenge)
	if !ok {
		t.Fatal("accessController did not return a challenge")
	}

	if challenge.Error() != ErrTokenRequired.Error() {
		t.Fatalf("accessControler did not get expected error - got %s - expected %s", challenge, ErrTokenRequired)
	}

	if grant != nil {
		t.Fatalf("expected nil auth grant but got %#v", grant)
	}

	// 2. Supply an invalid token.
	invalidJwk, err := makeSigningKeyWithChain(rootKeys[1], 1)
	if err != nil {
		t.Fatal(err)
	}

	token, err := makeTestToken(
		invalidJwk, issuer, service,
		[]*ResourceActions{{
			Type:    testAccess.Type,
			Name:    testAccess.Name,
			Actions: []string{testAccess.Action},
		}},
		time.Now(), time.Now().Add(5*time.Minute), // Everything is valid except the key which signed it.
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Raw))

	grant, err = accessController.Authorized(req, testAccess)
	challenge, ok = err.(auth.Challenge)
	if !ok {
		t.Fatal("accessController did not return a challenge")
	}

	if challenge.Error() != ErrInvalidToken.Error() {
		t.Fatalf("accessControler did not get expected error - got %s - expected %s", challenge, ErrTokenRequired)
	}

	if grant != nil {
		t.Fatalf("expected nil auth grant but got %#v", grant)
	}

	// create a valid jwk
	jwk, err := makeSigningKeyWithChain(rootKeys[0], 1)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Supply a token with insufficient access.
	token, err = makeTestToken(
		jwk, issuer, service,
		[]*ResourceActions{}, // No access specified.
		time.Now(), time.Now().Add(5*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Raw))

	grant, err = accessController.Authorized(req, testAccess)
	challenge, ok = err.(auth.Challenge)
	if !ok {
		t.Fatal("accessController did not return a challenge")
	}

	if challenge.Error() != ErrInsufficientScope.Error() {
		t.Fatalf("accessControler did not get expected error - got %s - expected %s", challenge, ErrInsufficientScope)
	}

	if grant != nil {
		t.Fatalf("expected nil auth grant but got %#v", grant)
	}

	// 4. Supply the token we need, or deserve, or whatever.
	token, err = makeTestToken(
		jwk, issuer, service,
		[]*ResourceActions{{
			Type:    testAccess.Type,
			Name:    testAccess.Name,
			Actions: []string{testAccess.Action},
		}},
		time.Now(), time.Now().Add(5*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Raw))

	grant, err = accessController.Authorized(req, testAccess)
	if err != nil {
		t.Fatalf("accessController returned unexpected error: %s", err)
	}

	if grant.User.Name != "foo" {
		t.Fatalf("expected user name %q, got %q", "foo", grant.User.Name)
	}

	// 5. Supply a token with full admin rights, which is represented as "*".
	token, err = makeTestToken(
		jwk, issuer, service,
		[]*ResourceActions{{
			Type:    testAccess.Type,
			Name:    testAccess.Name,
			Actions: []string{"*"},
		}},
		time.Now(), time.Now().Add(5*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Raw))

	_, err = accessController.Authorized(req, testAccess)
	if err != nil {
		t.Fatalf("accessController returned unexpected error: %s", err)
	}
}

// This tests that newAccessController can handle PEM blocks in the certificate
// file other than certificates, for example a private key.
func TestNewAccessControllerPemBlock(t *testing.T) {
	rootKeys, err := makeRootKeys(2)
	if err != nil {
		t.Fatal(err)
	}

	rootCertBundleFilename, err := writeTempRootCerts(rootKeys)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(rootCertBundleFilename)

	// Add something other than a certificate to the rootcertbundle
	file, err := os.OpenFile(rootCertBundleFilename, os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := x509.MarshalECPrivateKey(rootKeys[0])
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(bytes)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	realm := "https://auth.example.com/token/"
	issuer := "test-issuer.example.com"
	service := "test-service.example.com"

	options := map[string]interface{}{
		"realm":          realm,
		"issuer":         issuer,
		"service":        service,
		"rootcertbundle": rootCertBundleFilename,
		"autoredirect":   false,
	}

	ac, err := newAccessController(options)
	if err != nil {
		t.Fatal(err)
	}

	if len(ac.(*accessController).rootCerts.Subjects()) != 2 { //nolint:staticcheck // FIXME(thaJeztah): ignore SA1019: ac.(*accessController).rootCerts.Subjects has been deprecated since Go 1.18: if s was returned by SystemCertPool, Subjects will not include the system roots. (staticcheck)
		t.Fatal("accessController has the wrong number of certificates")
	}
}
