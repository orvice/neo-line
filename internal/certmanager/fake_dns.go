package certmanager

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/google/uuid"
)

// fakeDNSZone models one authoritative DNS zone for tests.
type fakeDNSZone struct {
	name        string
	txt         map[string][]fakeTXTRecord // fqdn -> records
	cnames      map[string]string          // fqdn -> target
	cleanupFail bool
}

type fakeTXTRecord struct {
	id    string
	value string
}

// FakeDNSProvider implements DNS-01 with CNAME following and exact TXT cleanup.
type FakeDNSProvider struct {
	mu    sync.Mutex
	zones map[string]*fakeDNSZone // zone apex -> zone
	// PresentCalls and CleanUpCalls record invocations for assertions.
	PresentCalls []string
	CleanUpCalls []string
}

func NewFakeDNSProvider(zones ...*fakeDNSZone) *FakeDNSProvider {
	f := &FakeDNSProvider{zones: make(map[string]*fakeDNSZone)}
	for _, z := range zones {
		if z.txt == nil {
			z.txt = make(map[string][]fakeTXTRecord)
		}
		if z.cnames == nil {
			z.cnames = make(map[string]string)
		}
		f.zones[strings.TrimSuffix(strings.ToLower(z.name), ".")] = z
	}
	return f
}

func (f *FakeDNSProvider) Timeout() (timeout, interval time.Duration) {
	return time.Second, 50 * time.Millisecond
}

func (f *FakeDNSProvider) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	fqdn := f.resolveCNAMEFQDN(strings.TrimSuffix(strings.ToLower(info.FQDN), "."))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.PresentCalls = append(f.PresentCalls, fqdn)
	zone := f.findZoneFor(fqdn)
	if zone == nil {
		return errors.New("fake dns: zone not found")
	}
	rec := fakeTXTRecord{id: uuid.NewString(), value: info.Value}
	zone.txt[fqdn] = append(zone.txt[fqdn], rec)
	return nil
}

func (f *FakeDNSProvider) CleanUp(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	fqdn := f.resolveCNAMEFQDN(strings.TrimSuffix(strings.ToLower(info.FQDN), "."))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CleanUpCalls = append(f.CleanUpCalls, fqdn)
	zone := f.findZoneFor(fqdn)
	if zone == nil {
		return nil
	}
	if zone.cleanupFail {
		return errors.New("fake dns: cleanup failed")
	}
	records := zone.txt[fqdn]
	out := records[:0]
	for _, rec := range records {
		if rec.value != info.Value {
			out = append(out, rec)
		}
	}
	if len(out) == 0 {
		delete(zone.txt, fqdn)
	} else {
		zone.txt[fqdn] = out
	}
	return nil
}

func (f *FakeDNSProvider) findZoneFor(fqdn string) *fakeDNSZone {
	resolved := f.resolveCNAMEFQDN(fqdn)
	for apex, zone := range f.zones {
		if resolved == apex || strings.HasSuffix(resolved, "."+apex) {
			return zone
		}
		_ = apex
	}
	return nil
}

func (f *FakeDNSProvider) resolveCNAMEFQDN(fqdn string) string {
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[fqdn]; ok {
			return fqdn
		}
		seen[fqdn] = struct{}{}
		var target string
		for _, zone := range f.zones {
			if t, ok := zone.cnames[fqdn]; ok {
				target = strings.TrimSuffix(strings.ToLower(t), ".")
				break
			}
		}
		if target == "" {
			return fqdn
		}
		fqdn = target
	}
}

func (f *FakeDNSProvider) TXTValues(fqdn string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	zone := f.findZoneFor(fqdn)
	if zone == nil {
		return nil
	}
	resolved := f.resolveCNAMEFQDN(strings.TrimSuffix(strings.ToLower(fqdn), "."))
	var out []string
	for _, rec := range zone.txt[resolved] {
		out = append(out, rec.value)
	}
	return out
}
