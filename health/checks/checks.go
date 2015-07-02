package checks

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/docker/distribution/health"
	"golang.org/x/net/icmp"
	"golang.org/x/net/internal/iana"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const timeout time.Duration = 3 * time.Second

// FileChecker checks the existence of a file and returns and error
// if the file exists, taking the application out of rotation
func FileChecker(f string) health.Checker {
	return health.CheckFunc(func() error {
		if _, err := os.Stat(f); err == nil {
			return errors.New("file exists")
		}
		return nil
	})
}

// HTTPChecker does a HEAD request and verifies if the HTTP status
// code return is a 200, taking the application out of rotation if
// otherwise
func HTTPChecker(r string) health.Checker {
	return health.CheckFunc(func() error {
		response, err := http.Head(r)
		if err != nil {
			return errors.New("error while checking: " + r)
		}
		if response.StatusCode != http.StatusOK {
			return errors.New("downstream service returned unexpected status: " + string(response.StatusCode))
		}
		return nil
	})
}

// PingChecker sends an ICMP Echo request and verifies that a
// response was received within the maximum round-trip time,
// taking the application out of rotation otherwise. The maxRTT
// must be less than 3 seconds
func PingChecker(a string, maxRTT time.Duration) health.Checker {
	return health.CheckFunc(func() error {
		ip := net.ParseIP(a)
		if ip == nil {
			return fmt.Errorf("%s is not a valid IP address", a)
		}
		b := ip.To4()
		pingFunc := pingIPv4
		if b == nil {
			pingFunc = pingIPv6
		}
		rtt, err := pingFunc(ip)
		if err != nil {
			return fmt.Errorf("ping request unsuccessful: %v", err)
		}
		if rtt > maxRTT {
			return fmt.Errorf("maximum RTT of %v exceeded. measured RTT: %v", maxRTT, rtt)
		}
		return nil
	})
}

func pingIPv4(ip net.IP) (time.Duration, error) {
	c, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return 0, err
	}
	defer c.Close()

	wm := icmp.Message{
		Type: ipv4.ICMPTypeEcho, Code: 0,
		Body: &icmp.Echo{
			ID: os.Getpid() & 0xffff, Seq: 1,
			Data: []byte("I CAN HAZ REPLY?"),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return 0, err
	}
	if _, err := c.WriteTo(wb, &net.UDPAddr{IP: ip}); err != nil {
		return 0, err
	}

	rb := make([]byte, 1500)
	start := time.Now()
	deadline := start.Add(timeout)
	c.SetReadDeadline(deadline)
	n, _, err := c.ReadFrom(rb)
	if err != nil {
		return 0, err
	}
	rtt := time.Since(start)
	rm, err := icmp.ParseMessage(iana.ProtocolICMP, rb[:n])
	if err != nil {
		return 0, err
	}
	switch rm.Type {
	case ipv4.ICMPTypeEchoReply:
		return rtt, nil
	default:
		return 0, fmt.Errorf("got %v; wanted an echo reply", rm)
	}
}

func pingIPv6(ip net.IP) (time.Duration, error) {
	c, err := icmp.ListenPacket("udp6", "::")
	if err != nil {
		return 0, err
	}
	defer c.Close()

	wm := icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest, Code: 0,
		Body: &icmp.Echo{
			ID: os.Getpid() & 0xffff, Seq: 1,
			Data: []byte("I CAN HAZ REPLY?"),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return 0, err
	}
	if _, err := c.WriteTo(wb, &net.UDPAddr{IP: ip}); err != nil {
		return 0, err
	}

	rb := make([]byte, 1500)
	start := time.Now()
	deadline := start.Add(timeout)
	c.SetReadDeadline(deadline)
	n, _, err := c.ReadFrom(rb)
	if err != nil {
		return 0, err
	}
	rtt := time.Since(start)
	rm, err := icmp.ParseMessage(iana.ProtocolIPv6ICMP, rb[:n])
	if err != nil {
		return 0, err
	}
	switch rm.Type {
	case ipv6.ICMPTypeEchoReply:
		return rtt, nil
	default:
		return 0, fmt.Errorf("got %v; wanted an echo reply", rm)
	}

}
