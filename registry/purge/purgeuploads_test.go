package purge

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		config string
		want   *PurgeOption
		err    string
	}{
		{
			config: `
version: 0.1
storage:
  s3:
  maintenance:
    uploadpurging:
      enabled: true
      age: 120h
      interval: 48h
      dryrun: false`,
			want: &PurgeOption{
				Enabled:  true,
				DryRun:   false,
				Age:      120 * time.Hour,
				Interval: 48 * time.Hour,
			},
			err: "",
		},
		{
			config: `
version: 0.1
storage:
  s3:
  maintenance:
    uploadpurging:
      dryrun: false`,
			want: &PurgeOption{
				Enabled:  false,
				DryRun:   false,
				Age:      168 * time.Hour,
				Interval: 24 * time.Hour,
			},
			err: "",
		},
		{
			config: `
version: 0.1
storage:
  s3:
  maintenance:
    uploadpurging:
      enabled: true
      age: aaaa`,
			want: &PurgeOption{},
			err:  "age",
		},
		{
			config: `
version: 0.1
storage:
  s3:
  maintenance:
    uploadpurging:
      enabled: true
      interval: aaaa`,
			want: &PurgeOption{},
			err:  "interval",
		},
	}

	for _, tc := range tests {
		fp := strings.NewReader(tc.config)
		config, err := configuration.Parse(fp)
		if err != nil {
			t.Fatalf("failed to parse config file: %s: %s is not a valid config file", err, tc.config)
		}

		// here we are sure that the config data is valid and we can get the storage.maintenance.uploadpurging value and call type assertion.
		purgeConfig := config.Storage["maintenance"]["uploadpurging"].(map[interface{}]interface{})
		got, err := ParseConfig(purgeConfig)
		if err != nil && tc.err == "" {
			// got en unexception error
			t.Fatalf("failed to parse %+v: %+v", purgeConfig, err)
		} else if tc.err != "" && err == nil {
			t.Fatalf("should get a parse error for %+v", purgeConfig)
		} else if err != nil && tc.err != "" && strings.Index(err.Error(), tc.err) < 0 {
			t.Fatalf("error should contain string %s, but can't fint the string for error %s", tc.err, err.Error())
		}

		if tc.err == "" && !reflect.DeepEqual(tc.want, got) {
			t.Fatalf("expected: %v, got: %v", tc.want, got)
		}
	}
}
