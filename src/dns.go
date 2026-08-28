package src

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	dnsOnce     sync.Once
	fallbackDNS = []string{
		"1.1.1.1:53",
		"8.8.8.8:53",
		"1.0.0.1:53",
		"8.8.4.4:53",
		"9.9.9.9:53",
	}
)

func init() {
	InitDNSResolver()
}

// InitDNSResolver configures net.DefaultResolver with robust fallbacks for Android/Termux and minimal Linux environments.
func InitDNSResolver() {
	dnsOnce.Do(func() {
		var customDNS []string

		// 1. Discover Android system DNS properties via getprop (present on all Android devices)
		for _, prop := range []string{"net.dns1", "net.dns2", "net.dns3"} {
			if out, err := exec.Command("/system/bin/getprop", prop).Output(); err == nil {
				ip := strings.TrimSpace(string(out))
				if ip != "" && !strings.HasPrefix(ip, "127.") && ip != "::1" {
					if !strings.Contains(ip, ":") {
						customDNS = append(customDNS, ip+":53")
					} else {
						customDNS = append(customDNS, "["+ip+"]:53")
					}
				}
			}
		}

		// 2. Discover Termux $PREFIX/etc/resolv.conf if present
		if prefix := os.Getenv("PREFIX"); prefix != "" {
			if data, err := os.ReadFile(prefix + "/etc/resolv.conf"); err == nil {
				for line := range strings.SplitSeq(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "nameserver") {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							ip := fields[1]
							if ip != "" && !strings.HasPrefix(ip, "127.") && ip != "::1" {
								customDNS = append(customDNS, ip+":53")
							}
						}
					}
				}
			}
		}

		servers := append(customDNS, fallbackDNS...)

		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: 4 * time.Second,
				}

				// If system resolution provides an address that isn't localhost loopback, try it first
				if address != "" && !strings.HasPrefix(address, "127.") && !strings.HasPrefix(address, "[::1]") {
					if c, err := d.DialContext(ctx, network, address); err == nil {
						return c, nil
					}
				}

				// Fallback to discovered Android DNS and reliable public DNS resolvers
				for _, srv := range servers {
					if c, err := d.DialContext(ctx, "udp", srv); err == nil {
						return c, nil
					}
				}

				// Final attempt with system default if all fallbacks failed
				return d.DialContext(ctx, network, address)
			},
		}
	})
}
