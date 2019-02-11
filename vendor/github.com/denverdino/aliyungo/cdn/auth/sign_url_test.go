package auth

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"
)

var (
	testSignTime = time.Unix(1541064730, 0)
	testPrivKey  = "12345678"
)

func assertEqual(t *testing.T, name string, x, y interface{}) {
	if !reflect.DeepEqual(x, y) {
		t.Errorf("%s: Not equal! Expected='%v', Actual='%v'\n", name, x, y)
		t.FailNow()
	}
}

func TestAtypeAuth(t *testing.T) {
	r, _ := url.Parse("https://example.com/a?foo=bar")
	url := aTypeTest(r, testPrivKey, testSignTime)
	assertEqual(t, "testTypeA", "https://example.com/a?foo=bar&auth_key=1541064730-0-0-f9dd5ed1e274ab4b1d5f5745344bf28b", url)
}

func TestBtypeAuth(t *testing.T) {
	signer := NewURLSigner("b", testPrivKey)
	url, _ := signer.Sign("https://example.com/a?foo=bar", testSignTime)
	assertEqual(t, "testTypeB", "https://example.com/201811011732/3a19d83a89ccb00a73212420791b0123/a?foo=bar", url)
}

func TestCtypeAuth(t *testing.T) {
	signer := NewURLSigner("c", testPrivKey)
	url, _ := signer.Sign("https://example.com/a?foo=bar", testSignTime)
	assertEqual(t, "testTypeC", "https://example.com/7d6b308ce87beb16d9dba32d741220f6/5bdac81a/a?foo=bar", url)
}

func aTypeTest(r *url.URL, privateKey string, expires time.Time) string {
	//rand equals "0" in test case
	rand := "0"
	uid := "0"
	secret := fmt.Sprintf("%s-%d-%s-%s-%s", r.Path, expires.Unix(), rand, uid, privateKey)
	hashValue := md5.Sum([]byte(secret))
	authKey := fmt.Sprintf("%d-%s-%s-%x", expires.Unix(), rand, uid, hashValue)
	if r.RawQuery == "" {
		return fmt.Sprintf("%s?auth_key=%s", r.String(), authKey)
	}
	return fmt.Sprintf("%s&auth_key=%s", r.String(), authKey)
}
