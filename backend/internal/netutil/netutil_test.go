package netutil

import (
	"net"
	"reflect"
	"testing"
)

func addrs(items ...string) []net.Addr {
	out := make([]net.Addr, 0, len(items))
	for _, s := range items {
		out = append(out, &net.IPNet{IP: net.ParseIP(s)})
	}
	return out
}

func TestURLsForWildcard(t *testing.T) {
	got := urlsFor(addrs("192.168.1.5", "10.0.0.2", "127.0.0.1", "fe80::1", "::1", "fd00::1", "2408::1"), "0.0.0.0", 8080)
	want := []string{"http://10.0.0.2:8080", "http://192.168.1.5:8080", "http://[fd00::1]:8080"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("urlsFor = %v, want %v", got, want)
	}
}

func TestURLsForEmptyHostIsWildcard(t *testing.T) {
	if got := urlsFor(addrs("192.168.1.5"), "", 8080); len(got) != 1 || got[0] != "http://192.168.1.5:8080" {
		t.Fatalf("urlsFor empty host = %v", got)
	}
}

func TestURLsForSpecificHost(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"127.0.0.1", "http://127.0.0.1:8080"},
		{"localhost", "http://localhost:8080"},
		{"192.168.1.5", "http://192.168.1.5:8080"},
	}
	for _, c := range cases {
		got := urlsFor(addrs("192.168.1.9"), c.host, 8080)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("urlsFor(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestURLsForDedup(t *testing.T) {
	got := urlsFor(addrs("192.168.1.5", "192.168.1.5"), "0.0.0.0", 8080)
	if len(got) != 1 {
		t.Fatalf("urlsFor dedup = %v, want single", got)
	}
}

func TestURLsForLocalhostFallback(t *testing.T) {
	got := urlsFor(addrs("127.0.0.1", "fe80::1", "::1", "2408::1"), "0.0.0.0", 8080)
	want := []string{"http://localhost:8080"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("urlsFor fallback = %v, want %v", got, want)
	}
}
