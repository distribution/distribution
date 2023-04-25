package azure

import (
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

const (
	defaultRealm = "core.windows.net"
)

type Credentials struct {
	Type     string `mapstructure:"type"`
	ClientID string `mapstructure:"clientid"`
	TenantID string `mapstructure:"tenantid"`
	Secret   string `mapstructure:"secret"`
}

type Parameters struct {
	Container        string      `mapstructure:"container"`
	AccountName      string      `mapstructure:"accountname"`
	AccountKey       string      `mapstructure:"accountkey"`
	Credentials      Credentials `mapstructure:"credentials"`
	ConnectionString string      `mapstructure:"connectionstring"`
	Realm            string      `mapstructure:"realm"`
	RootDirectory    string      `mapstructure:"rootdirectory"`
	ServiceURL       string      `mapstructure:"serviceurl"`
}

func NewParameters(parameters map[string]interface{}) (*Parameters, error) {
	params := Parameters{
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
	return &params, nil
}
