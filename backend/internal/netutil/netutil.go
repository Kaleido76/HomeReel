package netutil

import (
	"net"
	"sort"
	"strconv"
)

// URLs returns the http URLs through which a service bound to host:port is
// reachable. Config keeps host at "0.0.0.0" in practice, so instead of
// forcing the operator to look up the machine's LAN IP, a wildcard bind
// enumerates this PC's addresses. IPv4 addresses are always included (they
// are what LAN devices use); IPv6 only covers unique-local (fc00::/7) —
// global addresses (e.g. ISP-assigned privacy addresses) are ephemeral and
// just noise. A specific bind address (e.g. "127.0.0.1") yields one URL.
func URLs(host string, port int) []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return urlsFor(addrs, host, port)
}

func urlsFor(addrs []net.Addr, host string, port int) []string {
	if !isWildcard(host) {
		return []string{"http://" + net.JoinHostPort(host, strconv.Itoa(port))}
	}
	var urls []string
	seen := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP
		if !ip.IsGlobalUnicast() || (ip.To4() == nil && !isULA(ip)) {
			continue
		}
		url := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port))
		if _, dup := seen[url]; dup {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
	}
	if len(urls) == 0 {
		urls = append(urls, "http://"+net.JoinHostPort("localhost", strconv.Itoa(port)))
	}
	sort.Strings(urls)
	return urls
}

// isWildcard reports whether host binds every interface ("0.0.0.0", "::" or empty).
func isWildcard(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

// isULA reports whether ip is an IPv6 unique-local address (fc00::/7).
func isULA(ip net.IP) bool {
	return len(ip) == 16 && ip[0]&0xfe == 0xfc
}
