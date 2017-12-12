package metadata

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func init() {
	fmt.Println("make sure your ecs is in vpc before you run ```go test```")
}

type MockMetaDataClient struct {
	I IMetaDataClient
}

func (vpc *MockMetaDataClient) Version(version string) IMetaDataClient {
	vpc.I.Version(version)
	return vpc
}

func (vpc *MockMetaDataClient) ResourceType(rtype string) IMetaDataClient {
	vpc.I.ResourceType(rtype)
	return vpc
}

func (vpc *MockMetaDataClient) Resource(resource string) IMetaDataClient {
	vpc.I.Resource(resource)
	return vpc
}

func (vpc *MockMetaDataClient) Url() (string, error) {
	return vpc.I.Url()
}

func (m *MockMetaDataClient) Go() ([]string, error) {
	uri, err := m.Url()
	if err != nil {
		return []string{}, errors.New("error retrieve url")
	}
	if strings.Contains(uri, HOSTNAME) {
		return []string{"hostname-test"}, nil
	}

	if strings.Contains(uri, DNS_NAMESERVERS) {
		return []string{"8.8.8.8", "8.8.4.4"}, nil
	}

	if strings.Contains(uri, EIPV4) {
		return []string{"1.1.1.1-test"}, nil
	}

	if strings.Contains(uri, IMAGE_ID) {
		return []string{"image-id-test"}, nil
	}

	if strings.Contains(uri, INSTANCE_ID) {
		return []string{"instanceid-test"}, nil
	}

	if strings.Contains(uri, MAC) {
		return []string{"mac-test"}, nil
	}

	if strings.Contains(uri, NETWORK_TYPE) {
		return []string{"network-type-test"}, nil
	}

	if strings.Contains(uri, OWNER_ACCOUNT_ID) {
		return []string{"owner-account-id-test"}, nil
	}

	if strings.Contains(uri, PRIVATE_IPV4) {
		return []string{"private-ipv4-test"}, nil
	}

	if strings.Contains(uri, REGION) {
		return []string{"region-test"}, nil
	}

	if strings.Contains(uri, SERIAL_NUMBER) {
		return []string{"serial-number-test"}, nil
	}

	if strings.Contains(uri, SOURCE_ADDRESS) {
		return []string{"source-address-test"}, nil
	}

	if strings.Contains(uri, VPC_CIDR_BLOCK) {
		return []string{"vpc-cidr-block-test"}, nil
	}

	if strings.Contains(uri, VPC_ID) {
		return []string{"vpc-id-test"}, nil
	}

	if strings.Contains(uri, VSWITCH_CIDR_BLOCK) {
		return []string{"vswitch-cidr-block-test"}, nil
	}

	if strings.Contains(uri, VSWITCH_ID) {
		return []string{"vswitch-id-test"}, nil
	}

	if strings.Contains(uri, NTP_CONF_SERVERS) {
		return []string{"ntp1.server.com", "ntp2.server.com"}, nil
	}

	if strings.Contains(uri, ZONE) {
		return []string{"zone-test"}, nil
	}

	return nil, errors.New("unknow resource error.")
}

func TestOK(t *testing.T) {
	fmt.Println("ok")
}

func NewMockMetaData(client *http.Client) *MetaData {
	if client == nil {
		client = &http.Client{}
	}
	return &MetaData{
		c: &MetaDataClient{client: client},
	}
}

//func NewMockMetaData(client *http.Client)* MetaData{
//	if client == nil {
//		client = &http.Client{}
//	}
//	return &MetaData{
//		c: &MockMetaDataClient{&MetaDataClient{client:client}},
//	}
//}

func TestHostname(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.HostName()
	if err != nil {
		t.Errorf("hostname err: %s", err.Error())
	}
	if host != "hostname-test" {
		t.Error("hostname not equal hostname-test")
	}
}

func TestEIPV4(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.EIPv4()
	if err != nil {
		t.Errorf("EIPV4 err: %s", err.Error())
	}
	if host != "1.1.1.1-test" {
		t.Error("EIPV4 not equal eipv4-test")
	}
}
func TestImageID(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.ImageID()
	if err != nil {
		t.Errorf("IMAGE_ID err: %s", err.Error())
	}
	if host != "image-id-test" {
		t.Error("IMAGE_ID not equal image-id-test")
	}
}
func TestInstanceID(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.InstanceID()
	if err != nil {
		t.Errorf("IMAGE_ID err: %s", err.Error())
	}
	if host != "instanceid-test" {
		t.Error("IMAGE_ID not equal instanceid-test")
	}
}
func TestMac(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.Mac()
	if err != nil {
		t.Errorf("Mac err: %s", err.Error())
	}
	if host != "mac-test" {
		t.Error("Mac not equal mac-test")
	}
}
func TestNetworkType(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.NetworkType()
	if err != nil {
		t.Errorf("NetworkType err: %s", err.Error())
	}
	if host != "network-type-test" {
		t.Error("networktype not equal network-type-test")
	}
}
func TestOwnerAccountID(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.OwnerAccountID()
	if err != nil {
		t.Errorf("owneraccountid err: %s", err.Error())
	}
	if host != "owner-account-id-test" {
		t.Error("owner-account-id not equal owner-account-id-test")
	}
}
func TestPrivateIPv4(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.PrivateIPv4()
	if err != nil {
		t.Errorf("privateIPv4 err: %s", err.Error())
	}
	if host != "private-ipv4-test" {
		t.Error("privateIPv4 not equal private-ipv4-test")
	}
}
func TestRegion(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.Region()
	if err != nil {
		t.Errorf("region err: %s", err.Error())
	}
	if host != "region-test" {
		t.Error("region not equal region-test")
	}
}
func TestSerialNumber(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.SerialNumber()
	if err != nil {
		t.Errorf("serial number err: %s", err.Error())
	}
	if host != "serial-number-test" {
		t.Error("serial number not equal serial-number-test")
	}
}

func TestSourceAddress(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.SourceAddress()
	if err != nil {
		t.Errorf("source address err: %s", err.Error())
	}
	if host != "source-address-test" {
		t.Error("source address not equal source-address-test")
	}
}
func TestVpcCIDRBlock(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.VpcCIDRBlock()
	if err != nil {
		t.Errorf("vpcCIDRBlock err: %s", err.Error())
	}
	if host != "vpc-cidr-block-test" {
		t.Error("vpc-cidr-block not equal vpc-cidr-block-test")
	}
}
func TestVpcID(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.VpcID()
	if err != nil {
		t.Errorf("vpcID err: %s", err.Error())
	}
	if host != "vpc-id-test" {
		t.Error("vpc-id not equal vpc-id-test")
	}
}
func TestVswitchCIDRBlock(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.VswitchCIDRBlock()
	if err != nil {
		t.Errorf("vswitchCIDRBlock err: %s", err.Error())
	}
	if host != "vswitch-cidr-block-test" {
		t.Error("vswitch-cidr-block not equal vswitch-cidr-block-test")
	}
}
func TestVswitchID(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.VswitchID()
	if err != nil {
		t.Errorf("vswitch id err: %s", err.Error())
	}
	if host != "vswitch-id-test" {
		t.Error("vswitch-id not equal vswitch-id-test")
	}
}
func TestNTPConfigServers(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.NTPConfigServers()
	if err != nil {
		t.Errorf("ntpconfigservers err: %s", err.Error())
	}
	if host[0] != "ntp1.server.com" || host[1] != "ntp2.server.com" {
		t.Error("ntp1.server.com not equal ntp1.server.com")
	}
}
func TestDNSServers(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.DNSNameServers()
	if err != nil {
		t.Errorf("dnsservers err: %s", err.Error())
	}
	if host[0] != "8.8.8.8" || host[1] != "8.8.4.4" {
		t.Error("dns servers not equal 8.8.8.8/8.8.4.4")
	}
}

func TestZone(t *testing.T) {
	meta := NewMockMetaData(nil)
	host, err := meta.Zone()
	if err != nil {
		t.Errorf("zone err: %s", err.Error())
	}
	if host != "zone-test" {
		t.Error("zone not equal zone-test")
	}
}
