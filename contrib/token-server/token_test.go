package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/docker/distribution/registry/auth"
	"github.com/docker/libtrust"
	"strings"
)

func TestCreateJWTSuccessWithEmptyACL(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pk, err := libtrust.FromCryptoPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	tokenIssuer := TokenIssuer{
		Expiration: time.Duration(100),
		Issuer:     "localhost",
		SigningKey: pk,
	}

	grantedAccessList := make([]auth.Access, 0, 0)
	token, err := tokenIssuer.CreateJWT("test", "test", grantedAccessList)

	tokens := strings.Split(token, ".")

	if len(token) == 0 {
		t.Fatal("token not generated.")
	}

	json, err := decodeJWT(tokens[1])
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(json, "test") {
		t.Fatal("Valid token was not generated.")
	}

}

func decodeJWT(rawToken string) (string, error) {
	data, err := joseBase64Decode(rawToken)
	if err != nil {
		return "", errors.New("Error in Decoding base64 String")
	}
	return data, nil
}

func joseBase64Decode(s string) (string, error) {
	switch len(s) % 4 {
	case 0:
	case 2:
		s += "=="
	case 3:
		s += "="
	default:
		{
			return "", errors.New("Invalid base64 String")
		}
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err //errors.New("Error in Decoding base64 String")
	}
	return string(data), nil
}
