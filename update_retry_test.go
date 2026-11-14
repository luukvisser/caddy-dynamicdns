package dynamicdns

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/libdns/libdns"
	"go.uber.org/zap"
)

// flakySetter is a libdns.RecordSetter whose SetRecords fails while fail is set.
type flakySetter struct {
	fail  bool
	calls int
}

func (f *flakySetter) SetRecords(_ context.Context, _ string, recs []libdns.Record) ([]libdns.Record, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("provider unavailable")
	}
	return recs, nil
}

// TestUpdateProviderDNSRetriesFailedUpdate verifies that a record whose update
// failed is not remembered as current, so the next check sends it again; once
// an update succeeds, an unchanged IP is not sent again.
func TestUpdateProviderDNSRetriesFailedUpdate(t *testing.T) {
	lastIPsMu.Lock()
	defer lastIPsMu.Unlock()
	lastIPs = nil
	seededNames = make(map[string]bool)
	t.Cleanup(func() {
		lastIPs = nil
		seededNames = make(map[string]bool)
	})

	setter := &flakySetter{fail: true}
	app := App{ctx: caddy.Context{}, logger: zap.NewNop()}
	p := &Provider{
		Domains:     map[string][]string{"example.com.": {"sub"}},
		dnsProvider: setter,
	}
	ip := []netip.Addr{netip.MustParseAddr("192.0.2.1")}

	app.updateProviderDNS(p, ip) // fails
	app.updateProviderDNS(p, ip) // must retry
	if setter.calls != 2 {
		t.Fatalf("after a failed update the next check should retry: %d SetRecords calls, want 2", setter.calls)
	}

	setter.fail = false
	app.updateProviderDNS(p, ip) // succeeds
	app.updateProviderDNS(p, ip) // IP unchanged: nothing to send
	if setter.calls != 3 {
		t.Fatalf("after a successful update an unchanged IP should not be sent again: %d SetRecords calls, want 3", setter.calls)
	}
}
