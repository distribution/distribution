package storage

import (
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestPlatformMustExist(t *testing.T) {
	platforms := []platform{{architecture: "amd64", os: "linux"}}

	tests := []struct {
		name       string
		platforms  []platform
		descriptor *v1.Platform
		expected   bool
	}{
		// No filter
		{
			name:       "no filter/nil platform",
			platforms:  nil,
			descriptor: nil,
			expected:   true,
		},
		{
			name:       "no filter/with platform",
			platforms:  nil,
			descriptor: &v1.Platform{Architecture: "amd64", OS: "linux"},
			expected:   true,
		},
		// With filter
		{
			name:       "with filter/nil platform",
			platforms:  platforms,
			descriptor: nil,
			expected:   false,
		},
		{
			name:       "with filter/matching platform",
			platforms:  platforms,
			descriptor: &v1.Platform{Architecture: "amd64", OS: "linux"},
			expected:   true,
		},
		{
			name:       "with filter/non-matching platform",
			platforms:  platforms,
			descriptor: &v1.Platform{Architecture: "arm64", OS: "linux"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &manifestListHandler{
				validateImageIndexes: validateImageIndexes{
					imagesExist:    true,
					imagePlatforms: tt.platforms,
				},
			}
			desc := v1.Descriptor{
				MediaType: v1.MediaTypeImageManifest,
				Digest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:      42,
				Platform:  tt.descriptor,
			}
			if got := handler.platformMustExist(desc); got != tt.expected {
				t.Errorf("platformMustExist() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
