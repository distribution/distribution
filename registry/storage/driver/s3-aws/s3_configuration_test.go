package s3

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestS3EffectiveRegion(t *testing.T) {
	isolateAWSConfig(t)
	for _, source := range []string{"missing", "environment", "shared config"} {
		t.Run(source, func(t *testing.T) {
			switch source {
			case "environment":
				t.Setenv("AWS_REGION", "us-east-2")
			case "shared config":
				filename := filepath.Join(t.TempDir(), "config")
				if err := os.WriteFile(filename, []byte("[default]\nregion = us-east-2\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_CONFIG_FILE", filename)
			}
			params := sdkTestParameters("https://storage.example")
			delete(params, "region")
			d, err := FromParameters(t.Context(), params)
			if source == "missing" {
				if err == nil || !strings.Contains(err.Error(), "region") {
					t.Fatalf("expected missing-region initialization error, got %v", err)
				}
				if _, err := New(t.Context(), DriverParameters{RegionEndpoint: "https://storage.example"}); err == nil {
					t.Fatal("New accepted an empty effective region")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := d.baseEmbed.Base.StorageDriver.(*driver).S3.Options().Region; got != "us-east-2" {
				t.Fatalf("effective region = %q", got)
			}
			params["region"] = "us-future-1"
			if got := driverFromParams(t, params).S3.Options().Region; got != "us-future-1" {
				t.Fatalf("explicit region = %q", got)
			}
		})
	}
}

func TestV4AuthCompatibility(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		name      string
		value     any
		wantError string
	}{
		{"omitted", nil, ""},
		{"null", nil, ""},
		{"true", true, ""},
		{"string true", "true", ""},
		{"false", false, "Signature Version 2"},
		{"string false", "false", "Signature Version 2"},
		{"invalid string", "invalid", "should be a boolean"},
		{"invalid type", 1, "should be a boolean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := sdkTestParameters("https://storage.example")
			if tc.name != "omitted" {
				params["v4auth"] = tc.value
			}
			d, err := FromParameters(t.Context(), params)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), "v4auth") || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected actionable v4auth error containing %q, got %v", tc.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			signed, err := d.RedirectURL(httptest.NewRequest("GET", "https://registry.example", nil), "/key")
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(signed)
			if err != nil {
				t.Fatal(err)
			}
			if u.Query().Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" || u.Query().Get("X-Amz-Signature") == "" {
				t.Fatal("accepted configuration did not produce a SigV4 redirect")
			}
		})
	}
}

func TestRequestChecksumPrecedence(t *testing.T) {
	isolateAWSConfig(t)
	for _, tc := range []struct {
		name, shared, environment, registry string
		want                                aws.RequestChecksumCalculation
	}{
		{"default", "", "", "", aws.RequestChecksumCalculationWhenRequired},
		{"shared", "when_supported", "", "", aws.RequestChecksumCalculationWhenSupported},
		{"environment", "when_required", "when_supported", "", aws.RequestChecksumCalculationWhenSupported},
		{"environment required", "when_supported", "when_required", "", aws.RequestChecksumCalculationWhenRequired},
		{"registry required", "when_supported", "when_supported", "when_required", aws.RequestChecksumCalculationWhenRequired},
		{"registry supported", "when_required", "when_required", "WHEN_SUPPORTED", aws.RequestChecksumCalculationWhenSupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_REQUEST_CHECKSUM_CALCULATION", tc.environment)
			if tc.shared != "" {
				filename := filepath.Join(t.TempDir(), "config")
				if err := os.WriteFile(filename, []byte("[profile test]\nrequest_checksum_calculation = "+tc.shared+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("AWS_CONFIG_FILE", filename)
				t.Setenv("AWS_PROFILE", "test")
			}
			params := sdkTestParameters("https://storage.example")
			if tc.registry != "" {
				params["requestchecksumcalculation"] = tc.registry
			}
			if got := driverFromParams(t, params).S3.Options().RequestChecksumCalculation; got != tc.want {
				t.Fatalf("request checksum calculation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequestChecksumValidation(t *testing.T) {
	for _, value := range []any{"", "off", "unsupported", true, 2} {
		params := sdkTestParameters("https://storage.example")
		params["requestchecksumcalculation"] = value
		if _, err := FromParameters(t.Context(), params); err == nil || !strings.Contains(err.Error(), "requestchecksumcalculation") {
			t.Errorf("policy=%v: expected configuration error, got %v", value, err)
		}
	}
	if _, err := New(t.Context(), DriverParameters{RequestChecksumCalculation: -1}); err == nil {
		t.Fatal("New accepted an invalid checksum policy")
	}
}
