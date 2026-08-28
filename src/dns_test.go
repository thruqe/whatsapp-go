package src

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDNSResolver(t *testing.T) {
	InitDNSResolver()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, "web.whatsapp.com")
	if err != nil {
		t.Fatalf("DNS lookup failed: %v", err)
	}

	if len(addrs) == 0 {
		t.Fatal("expected at least one IP address for web.whatsapp.com")
	}
	t.Logf("Resolved web.whatsapp.com to %v", addrs)
}
