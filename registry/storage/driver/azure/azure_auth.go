package azure

import (
	"context"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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

func newAzureClient(params *Parameters) (*azureClient, error) {
	if params.AccountKey != "" {
		cred, err := azblob.NewSharedKeyCredential(params.AccountName, params.AccountKey)
		if err != nil {
			return nil, err
		}
		client, err := azblob.NewClientWithSharedKeyCredential(params.ServiceURL, cred, nil)
		if err != nil {
			return nil, err
		}
		signer := &sharedKeySigner{
			cred: cred,
		}
		return &azureClient{
			container: params.Container,
			client:    client,
			signer:    signer,
		}, nil
	}

	var cred azcore.TokenCredential
	var err error
	if params.Credentials.Type == "client_secret" {
		creds := &params.Credentials
		if cred, err = azidentity.NewClientSecretCredential(creds.TenantID, creds.ClientID, creds.Secret, nil); err != nil {
			return nil, err
		}
	} else if cred, err = azidentity.NewDefaultAzureCredential(nil); err != nil {
		return nil, err
	}

	client, err := azblob.NewClient(params.ServiceURL, cred, nil)
	if err != nil {
		return nil, err
	}
	signer := &clientTokenSigner{
		client: client,
		cred:   cred,
	}
	return &azureClient{
		container: params.Container,
		client:    client,
		signer:    signer,
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
