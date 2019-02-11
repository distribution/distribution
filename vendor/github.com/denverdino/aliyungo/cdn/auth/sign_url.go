package auth

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"time"
)

// An URLSigner provides URL signing utilities to sign URLs for Aliyun CDN
// resources.
// authentication document: https://help.aliyun.com/document_detail/85117.html
type URLSigner struct {
	authType string
	privKey  string
}

// NewURLSigner returns a new signer object.
func NewURLSigner(authType string, privKey string) *URLSigner {
	return &URLSigner{
		authType: authType,
		privKey:  privKey,
	}
}

// Sign returns a signed aliyuncdn url based on authentication type
func (s URLSigner) Sign(uri string, expires time.Time) (string, error) {
	r, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("unable to parse url: %s", uri)
	}

	switch s.authType {
	case "a":
		return aTypeSign(r, s.privKey, expires), nil
	case "b":
		return bTypeSign(r, s.privKey, expires), nil
	case "c":
		return cTypeSign(r, s.privKey, expires), nil
	default:
		return "", fmt.Errorf("invalid authentication type")
	}
}

// sign by A type authentication method.
// authentication document: https://help.aliyun.com/document_detail/85113.html
func aTypeSign(r *url.URL, privateKey string, expires time.Time) string {
	//rand is a random uuid without "-"
	rand := GenerateUUID().String()
	// not use, "0" by default
	uid := "0"
	secret := fmt.Sprintf("%s-%d-%s-%s-%s", r.Path, expires.Unix(), rand, uid, privateKey)
	hashValue := md5.Sum([]byte(secret))
	authKey := fmt.Sprintf("%d-%s-%s-%x", expires.Unix(), rand, uid, hashValue)
	if r.RawQuery == "" {
		return fmt.Sprintf("%s?auth_key=%s", r.String(), authKey)
	}
	return fmt.Sprintf("%s&auth_key=%s", r.String(), authKey)

}

// sign by B type authentication method.
// authentication document: https://help.aliyun.com/document_detail/85114.html
func bTypeSign(r *url.URL, privateKey string, expires time.Time) string {
	formatExp := expires.Format("200601021504")
	secret := privateKey + formatExp + r.Path
	hashValue := md5.Sum([]byte(secret))
	signURL := fmt.Sprintf("%s://%s/%s/%x%s?%s", r.Scheme, r.Host, formatExp, hashValue, r.Path, r.RawQuery)
	return signURL
}

// sign by C type authentication method.
// authentication document: https://help.aliyun.com/document_detail/85115.html
func cTypeSign(r *url.URL, privateKey string, expires time.Time) string {
	hexExp := fmt.Sprintf("%x", expires.Unix())
	secret := privateKey + r.Path + hexExp
	hashValue := md5.Sum([]byte(secret))
	signURL := fmt.Sprintf("%s://%s/%x/%s%s?%s", r.Scheme, r.Host, hashValue, hexExp, r.Path, r.RawQuery)
	return signURL
}
