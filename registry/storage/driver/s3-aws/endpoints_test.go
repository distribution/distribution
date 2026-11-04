package s3

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func TestServiceEndpoints(t *testing.T) {
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_USE_FIPS_ENDPOINT", "false")
	t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "false")
	t.Setenv("AWS_S3_US_EAST_1_REGIONAL_ENDPOINT", "regional")
	t.Setenv("AWS_STS_REGIONAL_ENDPOINTS", "regional")

	baseParameters := map[string]any{
		"region": "us-east-1", "bucket": "registry-bucket", "forcepathstyle": true,
		"accesskey": "test-key", "secretkey": "test-secret",
	}

	for _, tc := range []struct {
		name                                string
		parameters                          map[string]any
		s3URL, stsURL                       string
		expectedS3Region, expectedSTSRegion string
	}{
		{
			name:  "us-east-1 defaults",
			s3URL: "https://s3.us-east-1.amazonaws.com", stsURL: "https://sts.us-east-1.amazonaws.com",
			expectedS3Region: "us-east-1", expectedSTSRegion: "us-east-1",
		},
		{
			name:       "shared regionendpoint",
			parameters: map[string]any{"regionendpoint": "https://minio.example"},
			s3URL:      "https://minio.example", stsURL: "https://minio.example",
			expectedS3Region: "us-east-1", expectedSTSRegion: "us-east-1",
		},
		{
			name: "service endpoint overrides",
			parameters: map[string]any{"serviceendpoints": map[string]any{
				"s3":  map[string]any{"url": "https://objects.example", "signingregion": "us-west-2"},
				"sts": map[string]any{"url": "https://identity.example", "signingregion": "us-east-2"},
			}},
			s3URL: "https://objects.example", stsURL: "https://identity.example",
			expectedS3Region: "us-west-2", expectedSTSRegion: "us-east-2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parameters := maps.Clone(baseParameters)
			for key, value := range tc.parameters {
				parameters[key] = value
			}
			drv, err := FromParameters(t.Context(), parameters)
			if err != nil {
				t.Fatal(err)
			}
			client := drv.baseEmbed.Base.StorageDriver.(*driver).S3
			sess, err := session.NewSession(&client.Config)
			if err != nil {
				t.Fatal(err)
			}
			sts := sess.ClientConfig("sts")
			if client.Endpoint != tc.s3URL || sts.Endpoint != tc.stsURL || sts.SigningRegion != tc.expectedSTSRegion {
				t.Fatalf("got S3 %q, STS %q (signing region %q); want S3 %q, STS %q (%q)",
					client.Endpoint, sts.Endpoint, sts.SigningRegion, tc.s3URL, tc.stsURL, tc.expectedSTSRegion)
			}
			req, _ := client.GetObjectRequest(&s3.GetObjectInput{Bucket: aws.String("registry-bucket"), Key: aws.String("blob")})
			req.SetContext(t.Context())
			if err := req.Sign(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(req.HTTPRequest.Header.Get("Authorization"), "/"+tc.expectedS3Region+"/s3/aws4_request") {
				t.Fatalf("S3 request was not signed for %s", tc.expectedS3Region)
			}
			if parameters["serviceendpoints"] != nil {
				parameters["regionendpoint"] = "https://minio.example"
				if _, err := FromParameters(t.Context(), parameters); err == nil || !strings.Contains(err.Error(), "cannot both be configured") {
					t.Fatalf("expected mutually exclusive endpoint settings to fail, got %v", err)
				}
			}
		})
	}
}
