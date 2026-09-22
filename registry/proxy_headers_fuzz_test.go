package registry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/distribution/configuration"
)

// FuzzProxyHeadersClientCert drives the real IP filter this fork adds on top of
// distribution (registry/proxy_headers.go).
//
// Threat model coverage (registry-threat-model.md): TM-10 / AS-10. In the
// registry module the listener is configured with tls.RequestClientCert, so the
// TLS stack asks for a client certificate but performs no verification of its
// own: r.TLS.PeerCertificates is whatever the peer chose to send. TLS proves
// possession of the private key for exactly one of them -- the leaf,
// PeerCertificates[0], which signs the handshake. Every other element is just
// bytes the peer attached.
//
// The decision this filter makes therefore has one correct form: trust the
// X-Forwarded-For header only when the leaf verifies against the configured CA.
// Accepting the request because some other element of the chain verifies lets a
// peer that holds no CA-issued key claim any source address, by attaching the
// public part of any certificate the Ingress CA ever issued.
func FuzzProxyHeadersClientCert(f *testing.F) {
	pool := newCertPool(f)

	caPath := filepath.Join(f.TempDir(), "ingress-client-ca.crt")
	if err := os.WriteFile(caPath, pool.ingressCAPEM, 0o600); err != nil {
		f.Fatalf("cannot write the CA file: %v", err)
	}

	// Indices into pool.certs, so a fuzzed byte selects a certificate.
	f.Add([]byte{}, false)
	f.Add([]byte{0}, false)          // the Ingress-issued leaf alone
	f.Add([]byte{1}, false)          // the peer's own leaf alone
	f.Add([]byte{1, 0}, false)       // own leaf, Ingress-issued cert attached
	f.Add([]byte{1, 0}, true)        // the same, with a CN requirement
	f.Add([]byte{0, 1}, false)       // Ingress-issued leaf first
	f.Add([]byte{1, 2}, false)       // own leaf, foreign CA cert attached
	f.Add([]byte{2, 0}, false)       // foreign leaf, Ingress-issued attached
	f.Add([]byte{1, 1, 1, 0}, false) // padding before the attached cert
	f.Add([]byte{3}, false)          // the CA certificate itself as a leaf
	f.Add([]byte{3, 0}, false)
	f.Add([]byte{0}, true)
	f.Add([]byte{4, 0}, false) // expired Ingress-issued leaf
	f.Add([]byte{5, 0}, false) // Ingress-issued but without client auth usage

	f.Fuzz(func(t *testing.T, chainSpec []byte, requireCN bool) {
		const (
			peerAddr      = "10.0.0.1:1234"
			forwardedAddr = "203.0.113.9"
			appliedAddr   = forwardedAddr + ":0"
		)

		// Keep an iteration cheap and bounded; a longer chain adds no new state.
		if len(chainSpec) > 8 {
			return
		}

		chain := make([]*x509.Certificate, 0, len(chainSpec))
		for _, index := range chainSpec {
			chain = append(chain, pool.certs[int(index)%len(pool.certs)])
		}

		config := &configuration.Configuration{}
		config.HTTP.RealIP.Enabled = true
		config.HTTP.RealIP.ClientCert.CA = caPath
		if requireCN {
			config.HTTP.RealIP.ClientCert.CN = pool.ingressLeafCN
		}

		var seen string
		handler := proxyHeadersHandler(context.Background(), config,
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = r.RemoteAddr
			}))

		request := httptest.NewRequest(http.MethodGet, "/v2/", nil)
		request.RemoteAddr = peerAddr
		request.Header.Set("X-Forwarded-For", forwardedAddr)
		if len(chain) > 0 {
			request.TLS = &tls.ConnectionState{PeerCertificates: chain}
		}

		handler.ServeHTTP(httptest.NewRecorder(), request)

		applied := seen != peerAddr
		if applied && seen != appliedAddr {
			t.Fatalf("the forwarded address was rewritten to %q, expected %q", seen, appliedAddr)
		}

		// Only the leaf carries proof of possession, so only the leaf may decide.
		want := len(chain) > 0 && pool.verifiesAsClient(chain[0]) &&
			(!requireCN || chain[0].Subject.CommonName == pool.ingressLeafCN)

		if applied != want {
			t.Fatalf("X-Forwarded-For was %s for a chain of %d certificate(s) whose leaf is %q; "+
				"trusting it must depend on the leaf alone, and the leaf %s verify against the CA",
				appliedOrNot(applied), len(chain), leafName(chain),
				didOrNot(len(chain) > 0 && pool.verifiesAsClient(chain[0])))
		}
	})
}

func appliedOrNot(applied bool) string {
	if applied {
		return "trusted"
	}
	return "ignored"
}

func didOrNot(ok bool) string {
	if ok {
		return "does"
	}
	return "does not"
}

func leafName(chain []*x509.Certificate) string {
	if len(chain) == 0 {
		return "<no certificate>"
	}
	return chain[0].Subject.CommonName
}

// certPool holds the certificates a peer could plausibly present, together with
// the CA the filter is configured to trust.
type certPool struct {
	ingressCAPEM  []byte
	ingressLeafCN string
	roots         *x509.CertPool
	certs         []*x509.Certificate
}

// verifiesAsClient mirrors the check the filter is meant to perform on the leaf.
func (p *certPool) verifiesAsClient(cert *x509.Certificate) bool {
	_, err := cert.Verify(x509.VerifyOptions{
		Roots:     p.roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	return err == nil
}

func newCertPool(f *testing.F) *certPool {
	f.Helper()

	ingressCA, ingressCAKey := issue(f, "ingress-client-ca", nil, nil, func(template *x509.Certificate) {
		template.IsCA = true
		template.KeyUsage = x509.KeyUsageCertSign
		template.BasicConstraintsValid = true
	})
	foreignCA, foreignCAKey := issue(f, "foreign-ca", nil, nil, func(template *x509.Certificate) {
		template.IsCA = true
		template.KeyUsage = x509.KeyUsageCertSign
		template.BasicConstraintsValid = true
	})

	clientAuth := func(template *x509.Certificate) {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	// Issued by the trusted CA: the certificate the Ingress controller presents.
	// Its public part is visible to anyone who observes a connection from it.
	ingressLeaf, _ := issue(f, "ingress-controller", ingressCA, ingressCAKey, clientAuth)

	// The peer's own key material, which the TLS handshake does prove possession
	// of, but which no trusted CA signed.
	ownLeaf, _ := issue(f, "peer-own-leaf", nil, nil, clientAuth)

	// Issued by an unrelated CA.
	foreignLeaf, _ := issue(f, "foreign-leaf", foreignCA, foreignCAKey, clientAuth)

	// Issued by the trusted CA but no longer valid.
	expiredLeaf, _ := issue(f, "expired-ingress-controller", ingressCA, ingressCAKey,
		func(template *x509.Certificate) {
			clientAuth(template)
			template.NotBefore = time.Now().Add(-48 * time.Hour)
			template.NotAfter = time.Now().Add(-24 * time.Hour)
		})

	// Issued by the trusted CA but not for client authentication.
	serverLeaf, _ := issue(f, "ingress-server-only", ingressCA, ingressCAKey,
		func(template *x509.Certificate) {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		})

	roots := x509.NewCertPool()
	roots.AddCert(ingressCA)

	return &certPool{
		ingressCAPEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ingressCA.Raw}),
		ingressLeafCN: ingressLeaf.Subject.CommonName,
		roots:         roots,
		// The order fixes the meaning of a fuzzed index.
		certs: []*x509.Certificate{
			ingressLeaf, // 0
			ownLeaf,     // 1
			foreignLeaf, // 2
			ingressCA,   // 3
			expiredLeaf, // 4
			serverLeaf,  // 5
		},
	}
}

// issue creates a certificate, self-signed when parent is nil.
func issue(f *testing.F, commonName string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey,
	customise func(*x509.Certificate),
) (*x509.Certificate, *ecdsa.PrivateKey) {
	f.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		f.Fatalf("cannot generate a key for %q: %v", commonName, err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		f.Fatalf("cannot generate a serial for %q: %v", commonName, err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	if customise != nil {
		customise(template)
	}

	signer, signerKey := parent, parentKey
	if signer == nil {
		signer, signerKey = template, key
	}

	der, err := x509.CreateCertificate(rand.Reader, template, signer, &key.PublicKey, signerKey)
	if err != nil {
		f.Fatalf("cannot create the certificate %q: %v", commonName, err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		f.Fatalf("cannot parse the certificate %q: %v", commonName, err)
	}

	return cert, key
}
