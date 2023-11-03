package requestutil

import (
	"net"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

func parseIP(ipStr string) net.IP {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Warnf("invalid remote IP address: %q", ipStr)
	}
	return ip
}

// RemoteAddr extracts the remote address of the request, taking into
// account proxy headers.
func RemoteAddr(r *http.Request) string {
	if prior := r.Header.Get("X-Forwarded-For"); prior != "" {
		remoteAddr, _, _ := strings.Cut(prior, ",")
		remoteAddr = strings.Trim(remoteAddr, " ")
		if parseIP(remoteAddr) != nil {
			return remoteAddr
		}
	}
	// X-Real-Ip is less supported, but worth checking in the
	// absence of X-Forwarded-For
	if realIP := r.Header.Get("X-Real-Ip"); realIP != "" {
		if parseIP(realIP) != nil {
			return realIP
		}
	}

	return r.RemoteAddr
}

// RemoteIP extracts the remote IP of the request, taking into
// account proxy headers.
func RemoteIP(r *http.Request) string {
	addr := RemoteAddr(r)

	// Try parsing it as "IP:port"
	if ip, _, err := net.SplitHostPort(addr); err == nil {
		return ip
	}

	return addr
}
