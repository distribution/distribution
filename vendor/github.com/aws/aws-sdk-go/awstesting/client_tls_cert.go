package awstesting

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
)

// NewTLSClientCertServer creates a new HTTP test server initialize to require
// HTTP clients authenticate with TLS client certificates.
func NewTLSClientCertServer(handler http.Handler) (*httptest.Server, error) {
	server := httptest.NewUnstartedServer(handler)

	if server.TLS == nil {
		server.TLS = &tls.Config{}
	}
	server.TLS.ClientAuth = tls.RequireAndVerifyClientCert

	if server.TLS.ClientCAs == nil {
		server.TLS.ClientCAs = x509.NewCertPool()
	}
	certPem := append(ClientTLSCert, ClientTLSKey...)
	if ok := server.TLS.ClientCAs.AppendCertsFromPEM(certPem); !ok {
		return nil, fmt.Errorf("failed to append client certs")
	}

	return server, nil
}

// CreateClientTLSCertFiles returns a set of temporary files for the client
// certificate and key files.
func CreateClientTLSCertFiles() (cert, key string, err error) {
	cert, err = createTmpFile(ClientTLSCert)
	if err != nil {
		return "", "", err
	}

	key, err = createTmpFile(ClientTLSKey)
	if err != nil {
		return "", "", err
	}

	return cert, key, nil
}

/*
Client certificate generation

# Create CA
openssl genrsa -aes256 -passout pass:xxxx -out ca.pass.key 4096
openssl rsa -passin pass:xxxx -in ca.pass.key -out ca.key
rm ca.pass.key
openssl req -new -x509 -days 3650 -key ca.key -out ca.pem

# Create key for client
openssl genrsa -aes256 -passout pass:xxxx -out 01-client.pass.key 4096
openssl rsa -passin pass:xxxx -in 01-client.pass.key -out 01-client.key
rm 01-client.pass.key

# create csr for client
openssl req -new -key 01-client.key -out 01-client.csr
openssl x509 -req -days 3650 -in 01-client.csr -CA ca.pem -CAkey ca.key -set_serial 01 -out 01-client.pem
cat 01-client.key 01-client.pem ca.pem > 01-client.full.pem
*/

var (
	// ClientTLSCert 01-client.pem
	ClientTLSCert = []byte(`-----BEGIN CERTIFICATE-----
MIIEjjCCAnYCAQEwDQYJKoZIhvcNAQEFBQAwDTELMAkGA1UEBhMCVVMwHhcNMjAx
MTI0MDAyODI5WhcNMzAxMTIyMDAyODI5WjANMQswCQYDVQQGEwJVUzCCAiIwDQYJ
KoZIhvcNAQEBBQADggIPADCCAgoCggIBAN8gy1UtBR73fCJ9JWIREfBtqW/+hfNn
ZyIu7bc4MTWoP1dYG3CVV+HALijfVNeFQaohXjWaUIaXAa4idtM1AAf+J8GADqHp
z4qnAoIWLqfWRwtFJyggB2tnzmFA/yxR2jlpe3yT/OL0aXtYgS9bVeH6nWWjNuAo
D6qlTGSB/7ns8iDUK0WRJsodRGPi8OHNm4q5Pxqbzfvzu2vmF66NcvNb/96yIngl
Sjv6CSTz16hbbmqQQJAXurjkOLbSFCYZ76D2pYmqS/hLpUlFH/Bd/BcVP/3H5INA
fodY9Rx1oXETNuC69QgLA+2zlhGmbICh+OexIqNb2RH6vwi7EV5/Y4v7CKwzypre
OgOtkYQiDjhG3CxB+8E4q5t43SKpft7KFUXWmTaxOxZr7gmuBZGV5Lxzg+NgnFnV
tkPCVxKYsSSdhs11z0Ne/BBsGXCw0YoJ7HacFuVCf//C/vqT7y2ivhao3oMlv3ae
HjfHi9WIsZbDBB37Kk4UFXlXO0WXijrH09wILDW3IQ65fYMUBIyKFt9hKGjWKfcg
BWuTgJ98eG+BmxP6PIWgZTo1XdWKcxPblidLkwU4OzzuHONSsoGL8eeTBC0WcUT0
5H3bSVbkYQObKHe4fxCVUC/xEPgQga0NBlXLq0Zr8UnNPio7Vip5pzJ99ma4t4PN
TnP6f2B1zrjLAgMBAAEwDQYJKoZIhvcNAQEFBQADggIBAB2ei7140M68glYQk9vp
oOUz+8Ibg9ogo7LCk95jsFxGvBJu62mFfa1s1NMsWq4QHK5XG5nkq4ZUues6dl/B
LpSv8uwIwV7aNgZathJLxb2M4s32xPUodGbfaeVbk08qyEGMCo4QE000Hace/iFZ
jbNT6rkU6Edv/gsvHkVkCouMTsZhpMHezyrnSBAyxwqU82QVHbC2ByEQFNJ+0rCJ
gAzcXuWI/6X3+LQSQ44Y0n7nj7Rx6YidtwCoFoQ1oIAdlt6LyUKTtEUa3uN9Cdb6
nO4VGNC5p4URImHTMdqxDn0xpTYw0q9P+hierZYViuCaEokNlaWNk2wGHBqRlgxv
ci2qox1GCtabhRGyWEUzC9N6coVQPh1xuay8oQB/oXzcwk8LnUaOdVgwhKya1fEt
MQrlS/Vsv6e18UQXN0OM3V6mUFa+5wu+C4Ly7XQJ6EUwYZ6LYqO5ypsfXr8GrS0p
32l5nB7r80Q6mjKCG6MB827rIqWQvfadUX5q0xizb/RDKk+SmqxnffY38WpqLWec
WpEghlkp2IYQFdg7WxoKXCpz1rv+BI28rowRkVeW6chGqO9zx6Sk/twosiamgRK1
s2MhHZnvl1x4h+uPsST2b4FAyzuDXB39g7pUnAq9XVhWA6J4ndFduIh8jmVWdZBg
KJTU5ZEXpuI0w7WDrPwaIUbU
-----END CERTIFICATE-----
`)

	// ClientTLSKey 01-client.key
	ClientTLSKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`)

	// ClientCA ca.pem
	ClientCA = []byte(`-----BEGIN CERTIFICATE-----
MIIEljCCAn4CCQDzkVB8uGX1GDANBgkqhkiG9w0BAQsFADANMQswCQYDVQQGEwJV
UzAeFw0yMDExMjQwMDI3NDlaFw0zMDExMjIwMDI3NDlaMA0xCzAJBgNVBAYTAlVT
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA6gfPPxYl/6n/GuPkQwiT
RTnH0mohDgBT8vA9RpE3ffI/qY8zO0rD/bHJIsN2neus+bFIciro5S/LxaZ2amx5
Y1WZPOeNKW2r73FfwlhyhQ6a9noiXJYnMqcT3Hn7FLeEMtt3kYkXJw+9k230mBhn
L1vWP0KXNoi0B3T0SCJGhJAdjV/tTsTOKVnzaxaRfeXH//+S7McbEAA6m902+VDy
tLGjyjB4Ed+AKeQg59FOn6/Q+NBaCDCGPK1+R0NgVreT8yI0tIgDaApZUuKl6Iid
QNEHuGt77o8jgxKh93PDiVsgbnfRIkpHLwrJM8aDEMI34lSTt21MH6hbvBq6nA84
HkK4oAhYhV+qK5/I3INjP6cEIuIgxYqxYpIY31zmqkF0g0BtDy20zIwLohNVdxiB
1tMBRl1c4A1G2E1oZG+0BhO10xCWvk3pR5cdOsJCB7SnwQV895V3R1R12HqKNW11
8e5e5Vef7GnAbACgZKQwZGRnpa7BiClC4j5BOUgN33G8mUK1j19/8fo7HOg+qOLk
WTp+u0Dr/12WKrJc+p413ltwhbbxtpTsBKnqeRvp628pT3YY1aUP5iC4Ph7bAN/1
ziMgaKA/97A3UWgTEmLwzrhIAPsMU/zDa3FhI0cY3dDHD10iz303mZRfC97F6c8C
25VXx8/3pqpoLfYHhh9HtR8CAwEAATANBgkqhkiG9w0BAQsFAAOCAgEAANq6OnTW
xzxzcjB4UY0+XlXtgqSUy9DAmAAgfbFy+dgBtsYb45vvkKWLVrDov/spYch3S61I
Ba7bNWoqTkuOVVXcZtvG8zA8oH8xrU/Fgt+MIDCEFxuWttvroVEsomWyBh903fSB
y5c+gj4XvM5aYuLfljEi0R6qJNANIyyfSZkj6qR2yYm+Q7zK6SBCTlEfNdwuJfzy
ef4GJLotvx2+my8/DnUN4isDCQIdndXXhk2jlkQX839J84xOdGg2LtfjJPv/yDoY
ZkXcZF939jgg1Y7ppMg0BwhgqgfYCEf063O0C3elX41TL53hEIpu6/Qc9BbfkuxD
OO4mH2fGNXOGFo/liU+vQ9WNYHfPur1DcaMF2cKkaiK8EU53i+INU/94infU57fE
o2q6Wyzk82ozuyFsauKpXIUY5AiP2ovoMPcIE9Rfg38LpNtRLW/mFPuPK8hoQYdl
BKI5TeWiX0SvzsqlrMP814uwhFe/0l7heVuiDTIh4+rzXew5v8JmsPjFWAQvaNL8
tCTTIWUmJSMLbnQeZocDgp/vQUrCgj0OUgt9ScfZfevnhsUz1KvKO6gXyJamcs0S
zPTgPDpOZoBCbJdkM3J02ypSyQou2HYW+6C2CRZF+E3/Ef98RUembqiu2djP03ma
qhpIGyqpydp464PMJJsCSGEwGA3SDMFhc5E=
-----END CERTIFICATE-----
`)
)
