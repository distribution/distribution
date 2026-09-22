package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

// LoadPEMPrivKeyFile reads a PEM encoded RSA private key from the file name.
// A new RSA private key will be returned if no error.
func LoadPEMPrivKeyFile(name string) (key *rsa.PrivateKey, err error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}

	defer func() {
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		} else if closeErr != nil {
			err = fmt.Errorf("close error: %v, original error: %w", closeErr, err)
		}
	}()

	return LoadPEMPrivKey(file)
}

// LoadPEMPrivKey reads a PEM encoded RSA private key from the io.Reader.
// A new RSA private key will be returned if no error.
func LoadPEMPrivKey(reader io.Reader) (*rsa.PrivateKey, error) {
	block, err := loadPem(reader)
	if err != nil {
		return nil, err
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// LoadEncryptedPEMPrivKey decrypts the PEM encoded private key using the
// password provided returning a RSA private key. If the PEM data is invalid,
// or unable to decrypt an error will be returned.
//
// Deprecated: RFC 1423 PEM encryption is insecure. Callers using encrypted
// keys should instead decrypt that payload externally and pass it to
// [LoadPEMPrivKey].
func LoadEncryptedPEMPrivKey(reader io.Reader, password []byte) (*rsa.PrivateKey, error) {
	block, err := loadPem(reader)
	if err != nil {
		return nil, err
	}

	decryptedBlock, err := x509.DecryptPEMBlock(block, password)
	if err != nil {
		return nil, err
	}

	return x509.ParsePKCS1PrivateKey(decryptedBlock)
}

// LoadPEMPrivKeyPKCS8 reads a PEM-encoded RSA private key in PKCS8 format from
// the given reader.
//
// x509.ParsePKCS8PrivateKey can return multiple key types and this API does
// not discern between them. Callers in need of the underlying value must
// obtain it via type assertion:
//
//	key, err := LoadPEMPrivKeyPKCS8(r)
//	if err != nil { /* ... */ }
//
//	switch key.(type) {
//	case *rsa.PrivateKey:
//		// ...
//	case *ecdsa.PrivateKey:
//		// ...
//	case ed25519.PrivateKey:
//		// ...
//	default:
//		panic("unrecognized private key type")
//	}
//
// See aforementioned API docs for a full list of possible key types.
//
// If calling code can opaquely handle the returned key as a crypto.Signer, use
// [LoadPEMPrivKeyPKCS8AsSigner] instead.
func LoadPEMPrivKeyPKCS8(reader io.Reader) (any, error) {
	block, err := loadPem(reader)
	if err != nil {
		return nil, fmt.Errorf("load pem: %v", err)
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8 key: %v", err)
	}

	return key, nil
}

var (
	_ crypto.Signer = (*rsa.PrivateKey)(nil)
	_ crypto.Signer = (*ecdsa.PrivateKey)(nil)
	_ crypto.Signer = (ed25519.PrivateKey)(nil)
)

// LoadPEMPrivKeyPKCS8AsSigner wraps [LoadPEMPrivKeyPKCS8] to expect a crypto.Signer.
func LoadPEMPrivKeyPKCS8AsSigner(reader io.Reader) (crypto.Signer, error) {
	key, err := LoadPEMPrivKeyPKCS8(reader)
	if err != nil {
		return nil, fmt.Errorf("load key: %v", err)
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("key of type %T is not a crypto.Signer", key)
	}

	return signer, nil
}

func loadPem(reader io.Reader) (*pem.Block, error) {
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(b)
	if block == nil {
		// pem.Decode will set block to nil if there is no PEM data in the input
		// the second parameter will contain the provided bytes that failed
		// to be decoded.
		return nil, fmt.Errorf("no valid PEM data provided")
	}

	return block, nil
}
