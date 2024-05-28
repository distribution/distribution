package middleware

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect" // used as a replacement for testify
	"sync"
	"testing"
	"time"
)

// Rather than pull in all of testify
func assertEqual(t *testing.T, x, y interface{}) {
	if !reflect.DeepEqual(x, y) {
		t.Errorf("%s: Not equal! Expected='%v', Actual='%v'\n", t.Name(), x, y)
		t.FailNow()
	}
}

type mockIPRangeHandler struct {
	data awsIPResponse
}

func (m mockIPRangeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.Marshal(m.data)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	if _, err := w.Write(bytes); err != nil {
		w.WriteHeader(500)
		return
	}
}

func newTestHandler(data awsIPResponse) *httptest.Server {
	return httptest.NewServer(mockIPRangeHandler{
		data: data,
	})
}

func serverIPRanges(server *httptest.Server) string {
	return fmt.Sprintf("%s/", server.URL)
}

func setupTest(data awsIPResponse) *httptest.Server {
	// This is a basic schema which only claims the exact ip
	// is in aws.
	server := newTestHandler(data)
	return server
}

func TestS3TryUpdate(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{IPV4Prefix: "123.231.123.231/32"},
		},
	})
	defer server.Close()

	ips, _ := newAWSIPs(context.Background(), serverIPRanges(server), time.Hour, nil)

	assertEqual(t, 1, len(ips.ipv4))
	assertEqual(t, 0, len(ips.ipv6))
}

func TestMatchIPV6(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		V6Prefixes: []prefixEntry{
			{IPV6Prefix: "ff00::/16"},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, nil)
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, true, ips.contains(net.ParseIP("ff00::")))
	assertEqual(t, 1, len(ips.ipv6))
	assertEqual(t, 0, len(ips.ipv4))
}

func TestMatchIPV4(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{IPV4Prefix: "192.168.0.0/24"},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, nil)
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.0")))
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.1")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.169.0.0")))
}

func TestMatchIPV4_2(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{
				IPV4Prefix: "192.168.0.0/24",
				Region:     "us-east-1",
			},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, nil)
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.0")))
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.1")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.169.0.0")))
}

func TestMatchIPV4WithRegionMatched(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{
				IPV4Prefix: "192.168.0.0/24",
				Region:     "us-east-1",
			},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, []string{"us-east-1"})
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.0")))
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.1")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.169.0.0")))
}

func TestMatchIPV4WithRegionMatch_2(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{
				IPV4Prefix: "192.168.0.0/24",
				Region:     "us-east-1",
			},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, []string{"us-west-2", "us-east-1"})
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.0")))
	assertEqual(t, true, ips.contains(net.ParseIP("192.168.0.1")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.169.0.0")))
}

func TestMatchIPV4WithRegionNotMatched(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{
				IPV4Prefix: "192.168.0.0/24",
				Region:     "us-east-1",
			},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, []string{"us-west-2"})
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, false, ips.contains(net.ParseIP("192.168.0.0")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.168.0.1")))
	assertEqual(t, false, ips.contains(net.ParseIP("192.169.0.0")))
}

func TestInvalidData(t *testing.T) {
	t.Parallel()
	// Invalid entries from aws should be ignored.
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{IPV4Prefix: "9000"},
			{IPV4Prefix: "192.168.0.0/24"},
		},
	})
	defer server.Close()

	ctx := context.Background()
	ips, _ := newAWSIPs(ctx, serverIPRanges(server), time.Hour, nil)
	err := ips.tryUpdate(ctx)
	assertEqual(t, err, nil)
	assertEqual(t, 1, len(ips.ipv4))
}

func TestInvalidNetworkType(t *testing.T) {
	t.Parallel()
	server := setupTest(awsIPResponse{
		Prefixes: []prefixEntry{
			{IPV4Prefix: "192.168.0.0/24"},
		},
		V6Prefixes: []prefixEntry{
			{IPV6Prefix: "ff00::/8"},
			{IPV6Prefix: "fe00::/8"},
		},
	})
	defer server.Close()

	ips, _ := newAWSIPs(context.Background(), serverIPRanges(server), time.Hour, nil)
	assertEqual(t, 0, len(ips.getCandidateNetworks(make([]byte, 17)))) // 17 bytes does not correspond to any net type
	assertEqual(t, 1, len(ips.getCandidateNetworks(make([]byte, 4))))  // netv4 networks
	assertEqual(t, 2, len(ips.getCandidateNetworks(make([]byte, 16)))) // netv6 networks
}

func TestParsing(t *testing.T) {
	data := `{
      "prefixes": [{
        "ip_prefix": "192.168.0.0",
        "region": "someregion",
        "service": "s3"}],
      "ipv6_prefixes": [{
        "ipv6_prefix": "2001:4860:4860::8888",
        "region": "anotherregion",
        "service": "ec2"}]
    }`
	rawMockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(data)); err != nil {
			w.WriteHeader(500)
			return
		}
	})
	t.Parallel()
	server := httptest.NewServer(rawMockHandler)
	defer server.Close()
	schema, err := fetchAWSIPs(context.Background(), server.URL)

	assertEqual(t, nil, err)
	assertEqual(t, 1, len(schema.Prefixes))
	assertEqual(t, prefixEntry{
		IPV4Prefix: "192.168.0.0",
		Region:     "someregion",
		Service:    "s3",
	}, schema.Prefixes[0])
	assertEqual(t, 1, len(schema.V6Prefixes))
	assertEqual(t, prefixEntry{
		IPV6Prefix: "2001:4860:4860::8888",
		Region:     "anotherregion",
		Service:    "ec2",
	}, schema.V6Prefixes[0])
}

func TestUpdateCalledRegularly(t *testing.T) {
	t.Parallel()

	mu := &sync.RWMutex{}
	updateCount := 0

	server := httptest.NewServer(http.HandlerFunc(
		func(rw http.ResponseWriter, req *http.Request) {
			mu.Lock()
			updateCount++
			mu.Unlock()
			if _, err := rw.Write([]byte("ok")); err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
			}
		}))
	defer server.Close()
	if _, err := newAWSIPs(context.Background(), fmt.Sprintf("%s/", server.URL), time.Second, nil); err != nil {
		t.Fatalf("failed creating AWS IP filter: %v", err)
	}
	time.Sleep(time.Second*4 + time.Millisecond*500)
	mu.RLock()
	defer mu.RUnlock()
	if updateCount < 4 {
		t.Errorf("Update should have been called at least 4 times, actual=%d", updateCount)
	}
}

func TestEligibleForS3(t *testing.T) {
	t.Parallel()
	ips := &awsIPs{
		ipv4: []net.IPNet{{
			IP:   net.ParseIP("192.168.1.1"),
			Mask: net.IPv4Mask(255, 255, 255, 0),
		}},
		initialized: true,
	}

	tests := []struct {
		RemoteAddr string
		Expected   bool
	}{
		{RemoteAddr: "", Expected: false},
		{RemoteAddr: "192.168.1.2", Expected: true},
		{RemoteAddr: "192.168.0.2", Expected: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("Client IP = %v", tc.RemoteAddr), func(t *testing.T) {
			t.Parallel()
			req := &http.Request{RemoteAddr: tc.RemoteAddr}
			assertEqual(t, tc.Expected, eligibleForS3(req, ips))
		})
	}
}

func TestEligibleForS3WithAWSIPNotInitialized(t *testing.T) {
	t.Parallel()
	ips := &awsIPs{
		ipv4: []net.IPNet{{
			IP:   net.ParseIP("192.168.1.1"),
			Mask: net.IPv4Mask(255, 255, 255, 0),
		}},
		initialized: false,
	}

	tests := []struct {
		RemoteAddr string
		Expected   bool
	}{
		{RemoteAddr: "", Expected: false},
		{RemoteAddr: "192.168.1.2", Expected: false},
		{RemoteAddr: "192.168.0.2", Expected: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("Client IP = %v", tc.RemoteAddr), func(t *testing.T) {
			t.Parallel()
			req := &http.Request{RemoteAddr: tc.RemoteAddr}
			assertEqual(t, tc.Expected, eligibleForS3(req, ips))
		})
	}
}

// populate ips with a number of different ipv4 and ipv6 networks, for the purposes
// of benchmarking contains() performance.
func populateRandomNetworks(b *testing.B, ips *awsIPs, ipv4Count, ipv6Count int) {
	generateNetworks := func(dest *[]net.IPNet, bytes int, count int) {
		for i := 0; i < count; i++ {
			ip := make([]byte, bytes)
			_, err := rand.Read(ip)
			if err != nil {
				b.Fatalf("failed to generate network for test : %s", err.Error())
			}
			mask := make([]byte, bytes)
			for i := 0; i < bytes; i++ {
				mask[i] = 0xff
			}
			*dest = append(*dest, net.IPNet{
				IP:   ip,
				Mask: mask,
			})
		}
	}

	generateNetworks(&ips.ipv4, 4, ipv4Count)
	generateNetworks(&ips.ipv6, 16, ipv6Count)
}

func BenchmarkContainsRandom(b *testing.B) {
	// Generate a random network configuration, of size comparable to
	// aws official networks list
	// curl -s https://ip-ranges.amazonaws.com/ip-ranges.json | jq '.prefixes | length'
	// 941
	numNetworksPerType := 1000 // keep in sync with the above
	// intentionally skip constructor when creating awsIPs, to avoid updater routine.
	// This benchmark is only concerned with contains() performance.
	ips := awsIPs{}
	populateRandomNetworks(b, &ips, numNetworksPerType, numNetworksPerType)

	ipv4 := make([][]byte, b.N)
	ipv6 := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		ipv4[i] = make([]byte, 4)
		ipv6[i] = make([]byte, 16)
		_, err := rand.Read(ipv4[i])
		if err != nil {
			b.Fatal(err)
		}
		_, err = rand.Read(ipv6[i])
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ips.contains(ipv4[i])
		ips.contains(ipv6[i])
	}
}

func BenchmarkContainsProd(b *testing.B) {
	ips, _ := newAWSIPs(context.Background(), defaultIPRangesURL, defaultUpdateFrequency, nil)
	ipv4 := make([][]byte, b.N)
	ipv6 := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		ipv4[i] = make([]byte, 4)
		ipv6[i] = make([]byte, 16)
		_, err := rand.Read(ipv4[i])
		if err != nil {
			b.Fatal(err)
		}
		_, err = rand.Read(ipv6[i])
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ips.contains(ipv4[i])
		ips.contains(ipv6[i])
	}
}
