package purge

import (
	"fmt"
	"time"
)

// PurgeOption contains options for purging uploads
type PurgeOption struct {
	Enabled  bool
	Age      time.Duration
	Interval time.Duration
	DryRun   bool
}

func (po *PurgeOption) String() string {
	return fmt.Sprintf(`
Purge Option:
  Enabled:  %t
  DryRun:   %t
  Age:      %s
  Interval: %s
`, po.Enabled, po.DryRun, po.Age, po.Interval)
}

// UploadPurgeDefaultConfig provides a default configuration for upload
// purging to be used in the absence of configuration in the
// configuration file
func UploadPurgeDefaultConfig() map[interface{}]interface{} {
	config := map[interface{}]interface{}{}
	config["enabled"] = true
	config["age"] = "168h"
	config["interval"] = "24h"
	config["dryrun"] = false
	return config
}

func badPurgeUploadConfig(reason string) (*PurgeOption, error) {
	return nil, fmt.Errorf("Unable to parse upload purge configuration: %s", reason)
}

// ParseConfig will parse purge uploads configs and set default values if not set
func ParseConfig(config map[interface{}]interface{}) (*PurgeOption, error) {
	po := &PurgeOption{}

	if config["enabled"] == true {
		po.Enabled = true
	}

	var err error
	purgeAgeDuration := 168 * time.Hour
	purgeAge, ok := config["age"]
	if ok {
		ageStr, ok := purgeAge.(string)
		if !ok {
			return badPurgeUploadConfig("age is not a string")
		}
		purgeAgeDuration, err = time.ParseDuration(ageStr)
		if err != nil {
			return badPurgeUploadConfig(fmt.Sprintf("cannot parse age: %s", err.Error()))
		}
	}
	po.Age = purgeAgeDuration

	intervalDuration := 24 * time.Hour
	interval, ok := config["interval"]
	if ok {
		intervalStr, ok := interval.(string)
		if !ok {
			return badPurgeUploadConfig("interval is not a string")
		}

		intervalDuration, err = time.ParseDuration(intervalStr)
		if err != nil {
			return badPurgeUploadConfig(fmt.Sprintf("cannot parse interval: %s", err.Error()))
		}
	}
	po.Interval = intervalDuration

	var dryRunBool bool
	dryRun, ok := config["dryrun"]
	if ok {
		dryRunBool, ok = dryRun.(bool)
		if !ok {
			return badPurgeUploadConfig("cannot parse dryrun")
		}
		po.DryRun = dryRunBool
	} else {
		po.DryRun = false
	}

	return po, nil
}
