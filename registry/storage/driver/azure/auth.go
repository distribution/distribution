package azure

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

const (
	UDCGracePeriod = 30.0 * time.Minute
	UDCExpiryTime  = 48.0 * time.Hour
)

// signer abstracts the specifics of a blob SAS and is specialized
// for the different authentication credentials
type signer interface {
	Sign(context.Context, *sas.BlobSignatureValues) (sas.QueryParameters, error)
}

type sharedKeySigner struct {
	cred *azblob.SharedKeyCredential
}

type clientTokenSigner struct {
	client    *azblob.Client
	cred      azcore.TokenCredential
	udcMutex  sync.Mutex
	udc       *service.UserDelegationCredential
	udcExpiry time.Time
}

// azureClient abstracts signing blob urls for a container since the
// azure apis have completely different underlying authentication apis
type azureClient struct {
	container string
	client    *azblob.Client
	signer    signer
}

func newClient(params *DriverParameters) (*azureClient, error) {
	switch params.Credentials.Type {
	case CredentialsTypeClientSecret:
		return newTokenClient(params)
	case CredentialsTypeSharedKey, CredentialsTypeDefault:
		return newSharedKeyCredentialsClient(params)
	}
	return nil, fmt.Errorf("invalid credentials type: %q", params.Credentials.Type)
}

func newTokenClient(params *DriverParameters) (*azureClient, error) {
	var (
		cred azcore.TokenCredential
		err  error
	)

	switch params.Credentials.Type {
	case CredentialsTypeClientSecret:
		creds := &params.Credentials
		cred, err = azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.Secret, nil)
		if err != nil {
			return nil, fmt.Errorf("client secret credentials: %v", err)
		}
	default:
		cred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("default credentials: %v", err)
		}
	}

	azBlobOpts := &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			PerRetryPolicies: []policy.Policy{newRetryNotificationPolicy()},
			Logging: policy.LogOptions{
				AllowedHeaders: []string{
					"x-ms-error-code",
					"Retry-After",
					"Retry-After-Ms",
					"If-Match",
					"x-ms-blob-condition-appendpos",
				},
				AllowedQueryParams: []string{"comp"},
			},
		},
	}
	if params.SkipVerify {
		httpTransport := http.DefaultTransport.(*http.Transport).Clone()
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		azBlobOpts.Transport = &http.Client{
			Transport: httpTransport,
		}
	}
	client, err := azblob.NewClient(params.ServiceURL, cred, azBlobOpts)
	if err != nil {
		return nil, fmt.Errorf("new azure token client: %v", err)
	}

	return &azureClient{
		container: params.Container,
		client:    client,
		signer: &clientTokenSigner{
			client: client,
			cred:   cred,
		},
	}, nil
}

func newSharedKeyCredentialsClient(params *DriverParameters) (*azureClient, error) {
	cred, err := azblob.NewSharedKeyCredential(params.AccountName, params.AccountKey)
	if err != nil {
		return nil, fmt.Errorf("shared key credentials: %v", err)
	}
	azBlobOpts := &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			PerRetryPolicies: []policy.Policy{newRetryNotificationPolicy()},
			Logging: policy.LogOptions{
				AllowedHeaders: []string{
					"x-ms-error-code",
					"Retry-After",
					"Retry-After-Ms",
					"If-Match",
					"x-ms-blob-condition-appendpos",
				},
				AllowedQueryParams: []string{"comp"},
			},
		},
	}
	if params.SkipVerify {
		httpTransport := http.DefaultTransport.(*http.Transport).Clone()
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		azBlobOpts.Transport = &http.Client{
			Transport: httpTransport,
		}
	}
	client, err := azblob.NewClientWithSharedKeyCredential(params.ServiceURL, cred, azBlobOpts)
	if err != nil {
		return nil, fmt.Errorf("new azure client with shared credentials: %v", err)
	}

	return &azureClient{
		container: params.Container,
		client:    client,
		signer: &sharedKeySigner{
			cred: cred,
		},
	}, nil
}

func (a *azureClient) ContainerClient() *container.Client {
	return a.client.ServiceClient().NewContainerClient(a.container)
}

func (a *azureClient) SignBlobURL(ctx context.Context, blobURL string, expires time.Time) (string, error) {
	urlParts, err := sas.ParseURL(blobURL)
	if err != nil {
		return "", err
	}
	perms := sas.BlobPermissions{Read: true}
	signatureValues := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     time.Now().UTC().Add(-10 * time.Second),
		ExpiryTime:    expires,
		Permissions:   perms.String(),
		ContainerName: urlParts.ContainerName,
		BlobName:      urlParts.BlobName,
	}
	urlParts.SAS, err = a.signer.Sign(ctx, &signatureValues)
	if err != nil {
		return "", err
	}
	return urlParts.String(), nil
}

func (s *sharedKeySigner) Sign(ctx context.Context, signatureValues *sas.BlobSignatureValues) (sas.QueryParameters, error) {
	return signatureValues.SignWithSharedKey(s.cred)
}

func (s *clientTokenSigner) refreshUDC(ctx context.Context) (*service.UserDelegationCredential, error) {
	s.udcMutex.Lock()
	defer s.udcMutex.Unlock()

	now := time.Now().UTC()
	if s.udc == nil || s.udcExpiry.Sub(now) < UDCGracePeriod {
		// reissue user delegation credential
		startTime := now.Add(-10 * time.Second)
		expiryTime := startTime.Add(UDCExpiryTime)
		info := service.KeyInfo{
			Start:  to.Ptr(startTime.UTC().Format(sas.TimeFormat)),
			Expiry: to.Ptr(expiryTime.UTC().Format(sas.TimeFormat)),
		}
		udc, err := s.client.ServiceClient().GetUserDelegationCredential(ctx, info, nil)
		if err != nil {
			return nil, err
		}
		s.udc = udc
		s.udcExpiry = expiryTime
	}
	return s.udc, nil
}

func (s *clientTokenSigner) Sign(ctx context.Context, signatureValues *sas.BlobSignatureValues) (sas.QueryParameters, error) {
	udc, err := s.refreshUDC(ctx)
	if err != nil {
		return sas.QueryParameters{}, err
	}
	return signatureValues.SignWithUserDelegation(udc)
}
