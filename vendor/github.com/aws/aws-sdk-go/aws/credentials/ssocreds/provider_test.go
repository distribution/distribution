//go:build go1.9
// +build go1.9

package ssocreds

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sso"
	"github.com/aws/aws-sdk-go/service/sso/ssoiface"
)

type mockClient struct {
	ssoiface.SSOAPI

	t *testing.T

	Output *sso.GetRoleCredentialsOutput
	Err    error

	ExpectedAccountID    string
	ExpectedAccessToken  string
	ExpectedRoleName     string
	ExpectedClientRegion string

	Response func(mockClient) (*sso.GetRoleCredentialsOutput, error)
}

func (m mockClient) GetRoleCredentialsWithContext(ctx aws.Context, params *sso.GetRoleCredentialsInput, _ ...request.Option) (*sso.GetRoleCredentialsOutput, error) {
	m.t.Helper()

	if len(m.ExpectedAccountID) > 0 {
		if e, a := m.ExpectedAccountID, aws.StringValue(params.AccountId); e != a {
			m.t.Errorf("expect %v, got %v", e, a)
		}
	}

	if len(m.ExpectedAccessToken) > 0 {
		if e, a := m.ExpectedAccessToken, aws.StringValue(params.AccessToken); e != a {
			m.t.Errorf("expect %v, got %v", e, a)
		}
	}

	if len(m.ExpectedRoleName) > 0 {
		if e, a := m.ExpectedRoleName, aws.StringValue(params.RoleName); e != a {
			m.t.Errorf("expect %v, got %v", e, a)
		}
	}

	if m.Response == nil {
		return &sso.GetRoleCredentialsOutput{}, nil
	}

	return m.Response(m)
}

func swapCacheLocation(dir string) func() {
	original := defaultCacheLocation
	defaultCacheLocation = func() string {
		return dir
	}
	return func() {
		defaultCacheLocation = original
	}
}

func swapNowTime(referenceTime time.Time) func() {
	original := nowTime
	nowTime = func() time.Time {
		return referenceTime
	}
	return func() {
		nowTime = original
	}
}

func TestProvider(t *testing.T) {
	restoreCache := swapCacheLocation("testdata")
	defer restoreCache()

	restoreTime := swapNowTime(time.Date(2021, 01, 19, 19, 50, 0, 0, time.UTC))
	defer restoreTime()

	cases := map[string]struct {
		Client    mockClient
		AccountID string
		Region    string
		RoleName  string
		StartURL  string

		ExpectedErr         bool
		ExpectedCredentials credentials.Value
		ExpectedExpire      time.Time
	}{
		"missing required parameter values": {
			StartURL:    "https://invalid-required",
			ExpectedErr: true,
		},
		"valid required parameter values": {
			Client: mockClient{
				ExpectedAccountID:    "012345678901",
				ExpectedRoleName:     "TestRole",
				ExpectedClientRegion: "us-west-2",
				ExpectedAccessToken:  "dGhpcyBpcyBub3QgYSByZWFsIHZhbHVl",
				Response: func(mock mockClient) (*sso.GetRoleCredentialsOutput, error) {
					return &sso.GetRoleCredentialsOutput{
						RoleCredentials: &sso.RoleCredentials{
							AccessKeyId:     aws.String("AccessKey"),
							SecretAccessKey: aws.String("SecretKey"),
							SessionToken:    aws.String("SessionToken"),
							Expiration:      aws.Int64(1611177743123),
						},
					}, nil
				},
			},
			AccountID: "012345678901",
			Region:    "us-west-2",
			RoleName:  "TestRole",
			StartURL:  "https://valid-required-only",
			ExpectedCredentials: credentials.Value{
				AccessKeyID:     "AccessKey",
				SecretAccessKey: "SecretKey",
				SessionToken:    "SessionToken",
				ProviderName:    ProviderName,
			},
			ExpectedExpire: time.Date(2021, 01, 20, 21, 22, 23, 0.123e9, time.UTC),
		},
		"expired access token": {
			StartURL:    "https://expired",
			ExpectedErr: true,
		},
		"api error": {
			Client: mockClient{
				ExpectedAccountID:    "012345678901",
				ExpectedRoleName:     "TestRole",
				ExpectedClientRegion: "us-west-2",
				ExpectedAccessToken:  "dGhpcyBpcyBub3QgYSByZWFsIHZhbHVl",
				Response: func(mock mockClient) (*sso.GetRoleCredentialsOutput, error) {
					return nil, fmt.Errorf("api error")
				},
			},
			AccountID:   "012345678901",
			Region:      "us-west-2",
			RoleName:    "TestRole",
			StartURL:    "https://valid-required-only",
			ExpectedErr: true,
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			tt.Client.t = t

			provider := &Provider{
				Client:    tt.Client,
				AccountID: tt.AccountID,
				RoleName:  tt.RoleName,
				StartURL:  tt.StartURL,
			}

			provider.Expiry.CurrentTime = nowTime

			credentials, err := provider.Retrieve()
			if (err != nil) != tt.ExpectedErr {
				t.Errorf("expect error: %v", tt.ExpectedErr)
			}

			if e, a := tt.ExpectedCredentials, credentials; !reflect.DeepEqual(e, a) {
				t.Errorf("expect %v, got %v", e, a)
			}

			if !tt.ExpectedExpire.IsZero() {
				if e, a := tt.ExpectedExpire, provider.ExpiresAt(); !e.Equal(a) {
					t.Errorf("expect %v, got %v", e, a)
				}
			}
		})
	}
}
