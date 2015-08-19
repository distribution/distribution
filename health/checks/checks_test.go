package checks

import (
	"testing"
	"time"
)

const (
	maxRTT        = 3000 * time.Millisecond
	impossibleRTT = 1 * time.Microsecond
)

func TestFileChecker(t *testing.T) {
	if err := FileChecker("/tmp").Check(); err == nil {
		t.Errorf("/tmp was expected as exists")
	}

	if err := FileChecker("NoSuchFileFromMoon").Check(); err != nil {
		t.Errorf("NoSuchFileFromMoon was expected as not exists, error:%v", err)
	}
}

func TestHTTPChecker(t *testing.T) {
	if err := HTTPChecker("https://www.google.cybertron").Check(); err == nil {
		t.Errorf("Google on Cybertron was expected as not exists")
	}

	if err := HTTPChecker("https://www.google.pt").Check(); err != nil {
		t.Errorf("Google at Portugal was expected as exists, error:%v", err)
	}
}

func TestIPChecker(t *testing.T) {
	if err := PingChecker("8.8.8.8", maxRTT).Check(); err != nil {
		t.Errorf("8.8.8.8 was expected as exists")
	}
	if err := PingChecker("254.254.254.254", maxRTT).Check(); err == nil {
		t.Errorf("254.254.254.254 was expected as not exists")
	}
	if err := PingChecker("8.8.8.8", impossibleRTT).Check(); err == nil {
		t.Errorf("8.8.8.8 was expected as exists but not within 1msec")
	}

}

func TestIPCheckerIPv6(t *testing.T) {
	t.Skip("Support for IPv6 requires it to be enabled")
	if err := PingChecker("2001:4860:4860::8888", maxRTT).Check(); err != nil {
		t.Errorf("2001:4860:4860::8888 was expected as exists: %v", err)
	}
	if err := PingChecker("2001:1ce:1ce::babe", maxRTT).Check(); err == nil {
		t.Errorf("2001:1ce:1ce:babe:dddd was expected as not exists: %v", err)
	}
	if err := PingChecker("2001:4860:4860::8888", impossibleRTT).Check(); err == nil {
		t.Errorf("8.8.8.8 was expected as exists but not within 1μs")
	}

}
