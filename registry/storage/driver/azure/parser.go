package azure

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

const (
	defaultRealm      = "core.windows.net"
	defaultMaxRetries = 5
	defaultRetryDelay = "100ms"
)

type CredentialsType string

const (
	CredentialsTypeClientSecret = "client_secret"
	CredentialsTypeSharedKey    = "shared_key"
	CredentialsTypeDefault      = "default_credentials"
)

type Credentials struct {
	Type     CredentialsType `mapstructure:"type"`
	ClientID string          `mapstructure:"clientid"`
	TenantID string          `mapstructure:"tenantid"`
	Secret   string          `mapstructure:"secret"`
}

type DriverParameters struct {
	Credentials      Credentials `mapstructure:"credentials"`
	Container        string      `mapstructure:"container"`
	AccountName      string      `mapstructure:"accountname"`
	AccountKey       string      `mapstructure:"accountkey"`
	ConnectionString string      `mapstructure:"connectionstring"`
	Realm            string      `mapstructure:"realm"`
	RootDirectory    string      `mapstructure:"rootdirectory"`
	ServiceURL       string      `mapstructure:"serviceurl"`
	MaxRetries       int         `mapstructure:"max_retries"`
	RetryDelay       string      `mapstructure:"retry_delay"`
	SkipVerify       bool        `mapstructure:"skipverify"`
}

func NewParameters(parameters map[string]interface{}) (*DriverParameters, error) {
	params := DriverParameters{
		Realm: defaultRealm,
	}
	if err := mapstructure.Decode(parameters, &params); err != nil {
		return nil, err
	}
	if params.AccountName == "" {
		return nil, errors.New("no accountname parameter provided")
	}
	if params.Container == "" {
		return nil, errors.New("no container parameter provider")
	}
	if params.ServiceURL == "" {
		params.ServiceURL = fmt.Sprintf("https://%s.blob.%s", params.AccountName, params.Realm)
	}
	if params.MaxRetries == 0 {
		params.MaxRetries = defaultMaxRetries
	}
	if params.RetryDelay == "" {
		params.RetryDelay = defaultRetryDelay
	}
	return &params, nil
}
