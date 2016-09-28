package ks3

import (
	"fmt"
)

// Region represents KS3 region
type Region struct {
	Name                string
	KS3Endpoint         string
	KS3InternalEndpoint string
	Protocl             string
	CurrentUseEndpoint  string
	RegionEndpoint      string
}

// Constants of region definition
var Hangzhou = Region{
	"ks3-cn-hangzhou",
	"kss.ksyun.com",
	"kss-internal.ksyun.com",
	"http",
	"",
	"",
}

var Beijing = Region{
	"ks3-cn-beijing",
	"ks3-cn-beijing.ksyun.com",
	"ks3-cn-beijing-internal.ksyun.com",
	"http",
	"",
	"",
}

var Shanghai = Region{
	"ks3-cn-shanghai",
	"ks3-cn-shanghai.ksyun.com",
	"ks3-cn-shanghai-internal.ksyun.com",
	"http",
	"",
	"",
}

var Hongkong = Region{
	"ks3-cn-hk-1",
	"ks3-cn-hk-1.ksyun.com",
	"ks3-cn-hk-1-internal.ksyun.com",
	"http",
	"",
	"",
}

var USWest1 = Region{
	"ks3-us-west-1",
	"ks3-us-west-1.ksyun.com",
	"ks3-us-west-1-internal.ksyun.com",
	"http",
	"",
	"",
}

var Regions = map[string]Region{
	Hangzhou.Name: Hangzhou,
	Beijing.Name:  Beijing,
	Shanghai.Name: Shanghai,
	Hongkong.Name: Hongkong,
	USWest1.Name:  USWest1,
}

// GetRegion return an validated Region by regionName
func GetRegion(regionName string) (region Region) {
	region = Regions[regionName]
	return
}

// SetProtocol overwrite default Protocl
func (r *Region) SetProtocol(secure bool) {
	if secure {
		r.Protocl = "https"
	} else {
		r.Protocl = "http"
	}
}

// SetCurrentUseEndpoint overwrite default CurrentUseEndpoint
func (r *Region) SetCurrentUseEndpoint(internal bool) {
	if internal {
		r.CurrentUseEndpoint = r.KS3InternalEndpoint
	} else {
		r.CurrentUseEndpoint = r.KS3Endpoint
	}
}

// SetRegionEndpoint overwrite default RegionEndpoint
func (r *Region) SetRegionEndpoint(regionEndpoint string) {
	// TODO(softlns) check regionEndpoint
	r.RegionEndpoint = regionEndpoint
}

// GetEndpoint returns endpoint of region
func (r *Region) GetEndpoint() string {
	return fmt.Sprintf("%s://%s", r.Protocl, r.CurrentUseEndpoint)
}

// GetBucketEndpoint returns bucketendpoint of region
func (r *Region) GetBucketEndpoint(bucket string) string {
	if bucket == "" {
		return r.GetEndpoint()
	}
	return fmt.Sprintf("%s://%s.%s", r.Protocl, bucket, r.CurrentUseEndpoint)
}
