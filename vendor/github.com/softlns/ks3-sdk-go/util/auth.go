package util

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type ServiceError interface {
	error
	ErrorCode() string
}

type Error struct {
	StatusCode int
	Type       string
	Code       string
	Message    string
	RequestId  string
}

func (err *Error) Error() string {
	return fmt.Sprintf("Type: %s, Code: %s, Message: %s",
		err.Type, err.Code, err.Message,
	)
}

func (err *Error) ErrorCode() string {
	return err.Code
}

// Auth type
type Auth struct {
	AccessKey, SecretKey string
	token                string
	expiration           time.Time
}

// Token returns an Auth token
func (a *Auth) Token() string {
	if a.token == "" {
		return ""
	}
	if time.Since(a.expiration) >= -30*time.Second { //in an ideal world this should be zero assuming the instance is synching it's clock
		auth, err := GetAuth("", "", "", time.Time{})
		if err == nil {
			*a = auth
		}
	}
	return a.token
}

// GetAuth creates an Auth based on either passed in credentials,
// environment information or instance based role credentials.
func GetAuth(accessKey string, secretKey, token string, expiration time.Time) (auth Auth, err error) {
	// First try passed in credentials
	if accessKey != "" && secretKey != "" {
		return Auth{accessKey, secretKey, token, expiration}, nil
	}

	// Next try to get auth from the environment
	auth, err = EnvAuth()
	if err == nil {
		// Found auth, return
		return
	}

	err = fmt.Errorf("No valid KS3 authentication found: %s", err)
	return auth, err
}

// EnvAuth creates an Auth based on environment information.
// The KS3_ACCESS_KEY_ID and KS3_SECRET_ACCESS_KEY environment
// variables are used.
func EnvAuth() (auth Auth, err error) {
	auth.AccessKey = os.Getenv("KS3_ACCESS_KEY_ID")
	if auth.AccessKey == "" {
		auth.AccessKey = os.Getenv("KS3_ACCESS_KEY")
	}

	auth.SecretKey = os.Getenv("KS3_SECRET_ACCESS_KEY")
	if auth.SecretKey == "" {
		auth.SecretKey = os.Getenv("KS3_SECRET_KEY")
	}
	if auth.AccessKey == "" {
		err = errors.New("KS3_ACCESS_KEY_ID or KS3_ACCESS_KEY not found in environment")
	}
	if auth.SecretKey == "" {
		err = errors.New("KS3_SECRET_ACCESS_KEY or KS3_SECRET_KEY not found in environment")
	}
	return
}
