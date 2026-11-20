package s3

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/stretchr/testify/require"
)

func TestNewRegions(t *testing.T) {
	t.Setenv("AWS_USE_FIPS_ENDPOINT", "false")
	t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "false")

	for _, tc := range []struct {
		name      string
		region    string
		endpoint  string
		fips      bool
		dualStack bool

		expectedS3Endpoint  string
		expectedSTSEndpoint string
	}{
		{
			name:   "sdk-v1",
			region: "us-east-1",

			expectedS3Endpoint:  "https://s3.amazonaws.com",
			expectedSTSEndpoint: "https://sts.amazonaws.com",
		},
		{
			name:   "sdk-v1-fips",
			region: "us-east-1",
			fips:   true,

			expectedS3Endpoint:  "https://s3-fips.us-east-1.amazonaws.com",
			expectedSTSEndpoint: "https://sts-fips.us-east-1.amazonaws.com",
		},
		{
			name:      "sdk-v1-ds",
			region:    "us-east-1",
			dualStack: true,

			expectedS3Endpoint:  "https://s3.dualstack.us-east-1.amazonaws.com",
			expectedSTSEndpoint: "https://sts.us-east-1.api.aws",
		},
		{
			name:   "sdk-v2",
			region: "us-isob-west-1",

			expectedS3Endpoint:  "https://s3.us-isob-west-1.sc2s.sgov.gov",
			expectedSTSEndpoint: "https://sts.us-isob-west-1.sc2s.sgov.gov",
		},
		{
			name:   "sdk-v2-fips",
			region: "us-isob-west-1",
			fips:   true,

			expectedS3Endpoint:  "https://s3-fips.us-isob-west-1.sc2s.sgov.gov",
			expectedSTSEndpoint: "https://sts-fips.us-isob-west-1.sc2s.sgov.gov",
		},
		{
			name:      "sdk-v2-ds",
			region:    "eusc-de-east-1",
			dualStack: true,

			expectedS3Endpoint:  "https://s3.eusc-de-east-1.api.amazonwebservices.eu",
			expectedSTSEndpoint: "https://sts.eusc-de-east-1.api.amazonwebservices.eu",
		},
		{
			name:     "custom",
			region:   "custom",
			endpoint: "https://minio.example",

			expectedS3Endpoint:  "https://minio.example",
			expectedSTSEndpoint: "https://minio.example",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv, err := FromParameters(t.Context(), map[string]any{
				"region":          tc.region,
				"regionendpoint":  tc.endpoint,
				"usefipsendpoint": tc.fips,
				"usedualstack":    tc.dualStack,

				"bucket":    "registry-bucket",
				"accesskey": "test-key",
				"secretkey": "test-secret",
			})
			require.NoError(t, err)

			client := drv.baseEmbed.Base.StorageDriver.(*driver).S3
			sess, err := session.NewSession(&client.Config)
			require.NoError(t, err)

			require.Equal(t, tc.expectedS3Endpoint, client.Endpoint)
			require.Equal(t, tc.region, client.SigningRegion)

			identity := sts.New(sess)
			require.Equal(t, tc.expectedSTSEndpoint, identity.Endpoint)
			require.Equal(t, tc.region, identity.SigningRegion)
		})
	}
}
