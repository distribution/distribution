package registry

import (
	"bufio"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/docker/distribution/configuration"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
)

// Tests to ensure nextProtos returns the correct protocols when:
// * config.HTTP.HTTP2.Disabled is not explicitly set => [h2 http/1.1]
// * config.HTTP.HTTP2.Disabled is explicitly set to false [h2 http/1.1]
// * config.HTTP.HTTP2.Disabled is explicitly set to true [http/1.1]
func TestNextProtos(t *testing.T) {
	config := &configuration.Configuration{}
	protos := nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"h2", "http/1.1"}) {
		t.Fatalf("expected protos to equal [h2 http/1.1], got %s", protos)
	}
	config.HTTP.HTTP2.Disabled = false
	protos = nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"h2", "http/1.1"}) {
		t.Fatalf("expected protos to equal [h2 http/1.1], got %s", protos)
	}
	config.HTTP.HTTP2.Disabled = true
	protos = nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"http/1.1"}) {
		t.Fatalf("expected protos to equal [http/1.1], got %s", protos)
	}
}

type registryTLSConfig struct {
	cipherSuites    []string
	certificatePath string
	privateKeyPath  string
	certificate     *tls.Certificate
}

func setupRegistry(tlsCfg *registryTLSConfig, addr string) (*Registry, error) {
	config := &configuration.Configuration{}
	// TODO: this needs to change to something ephemeral as the test will fail if there is any server
	// already listening on port 5000
	config.HTTP.Addr = addr
	config.HTTP.DrainTimeout = time.Duration(10) * time.Second
	if tlsCfg != nil {
		config.HTTP.TLS.CipherSuites = tlsCfg.cipherSuites
		config.HTTP.TLS.Certificate = tlsCfg.certificatePath
		config.HTTP.TLS.Key = tlsCfg.privateKeyPath
	}
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]interface{}{}}
	return NewRegistry(context.Background(), config)
}

func TestGracefulShutdown(t *testing.T) {
	registry, err := setupRegistry(nil, ":5000")
	if err != nil {
		t.Fatal(err)
	}

	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	// Wait for some unknown random time for server to start listening
	time.Sleep(3 * time.Second)

	// send incomplete request
	conn, err := net.Dial("tcp", "localhost:5000")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "GET /v2/ ")

	// send stop signal
	quit <- os.Interrupt
	time.Sleep(100 * time.Millisecond)

	// try connecting again. it shouldn't
	_, err = net.Dial("tcp", "localhost:5000")
	if err == nil {
		t.Fatal("Managed to connect after stopping.")
	}

	// make sure earlier request is not disconnected and response can be received
	fmt.Fprintf(conn, "HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "200 OK" {
		t.Error("response status is not 200 OK: ", resp.Status)
	}
	if body, err := ioutil.ReadAll(resp.Body); err != nil || string(body) != "{}" {
		t.Error("Body is not {}; ", string(body))
	}
}

func TestGetCipherSuite(t *testing.T) {
	resp, err := getCipherSuites([]string{"TLS_RSA_WITH_AES_128_CBC_SHA"})
	if err != nil || len(resp) != 1 || resp[0] != tls.TLS_RSA_WITH_AES_128_CBC_SHA {
		t.Errorf("expected cipher suite %q, got %q",
			"TLS_RSA_WITH_AES_128_CBC_SHA",
			strings.Join(getCipherSuiteNames(resp), ","),
		)
	}

	resp, err = getCipherSuites([]string{"TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_AES_128_GCM_SHA256"})
	if err != nil || len(resp) != 2 ||
		resp[0] != tls.TLS_RSA_WITH_AES_128_CBC_SHA || resp[1] != tls.TLS_AES_128_GCM_SHA256 {
		t.Errorf("expected cipher suites %q, got %q",
			"TLS_RSA_WITH_AES_128_CBC_SHA,TLS_AES_128_GCM_SHA256",
			strings.Join(getCipherSuiteNames(resp), ","),
		)
	}

	_, err = getCipherSuites([]string{"TLS_RSA_WITH_AES_128_CBC_SHA", "bad_input"})
	if err == nil {
		t.Error("did not return expected error about unknown cipher suite")
	}
}

func buildRegistryTLSConfig(name, keyType string, cipherSuites []string) (*registryTLSConfig, error) {
	var priv interface{}
	var pub crypto.PublicKey
	var err error
	switch keyType {
	case "rsa":
		priv, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to create rsa private key: %v", err)
		}
		rsaKey := priv.(*rsa.PrivateKey)
		pub = rsaKey.Public()
	case "ecdsa":
		priv, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create ecdsa private key: %v", err)
		}
		ecdsaKey := priv.(*ecdsa.PrivateKey)
		pub = ecdsaKey.Public()
	default:
		return nil, fmt.Errorf("unsupported key type: %v", keyType)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Minute)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to create serial number: %v", err)
	}
	cert := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"registry_test"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &cert, &cert, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %v", err)
	}
	if _, err := os.Stat(os.TempDir()); os.IsNotExist(err) {
		os.Mkdir(os.TempDir(), 1777)
	}

	certPath := path.Join(os.TempDir(), name+".pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create pem: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, fmt.Errorf("failed to write data to %s: %v", certPath, err)
	}
	if err := certOut.Close(); err != nil {
		return nil, fmt.Errorf("error closing %s: %v", certPath, err)
	}

	keyPath := path.Join(os.TempDir(), name+".key")
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s for writing: %v", keyPath, err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal private key: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return nil, fmt.Errorf("failed to write data to key.pem: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		return nil, fmt.Errorf("error closing %s: %v", keyPath, err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	tlsTestCfg := registryTLSConfig{
		cipherSuites:    cipherSuites,
		certificatePath: certPath,
		privateKeyPath:  keyPath,
		certificate:     &tlsCert,
	}

	return &tlsTestCfg, nil
}

func TestRegistrySupportedCipherSuite(t *testing.T) {
	name := "registry_test_server_supported_cipher"
	cipherSuites := []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}
	serverTLS, err := buildRegistryTLSConfig(name, "rsa", cipherSuites)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := setupRegistry(serverTLS, ":5001")
	if err != nil {
		t.Fatal(err)
	}

	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	// Wait for some unknown random time for server to start listening
	time.Sleep(3 * time.Second)

	// send tls request with server supported cipher suite
	clientCipherSuites, err := getCipherSuites(cipherSuites)
	if err != nil {
		t.Fatal(err)
	}
	clientTLS := tls.Config{
		InsecureSkipVerify: true,
		CipherSuites:       clientCipherSuites,
	}
	dialer := net.Dialer{
		Timeout: time.Second * 5,
	}
	conn, err := tls.DialWithDialer(&dialer, "tcp", "127.0.0.1:5001", &clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "GET /v2/ HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "200 OK" {
		t.Error("response status is not 200 OK: ", resp.Status)
	}
	if body, err := ioutil.ReadAll(resp.Body); err != nil || string(body) != "{}" {
		t.Error("Body is not {}; ", string(body))
	}

	// send stop signal
	quit <- os.Interrupt
	time.Sleep(100 * time.Millisecond)
}

func TestRegistryUnsupportedCipherSuite(t *testing.T) {
	name := "registry_test_server_unsupported_cipher"
	serverTLS, err := buildRegistryTLSConfig(name, "rsa", []string{"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA358"})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := setupRegistry(serverTLS, ":5002")
	if err != nil {
		t.Fatal(err)
	}

	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	// Wait for some unknown random time for server to start listening
	time.Sleep(3 * time.Second)

	// send tls request with server unsupported cipher suite
	clientTLS := tls.Config{
		InsecureSkipVerify: true,
		CipherSuites:       []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
	}
	dialer := net.Dialer{
		Timeout: time.Second * 5,
	}
	_, err = tls.DialWithDialer(&dialer, "tcp", "127.0.0.1:5002", &clientTLS)
	if err == nil {
		t.Error("expected TLS connection to timeout")
	}

	// send stop signal
	quit <- os.Interrupt
	time.Sleep(100 * time.Millisecond)
}
