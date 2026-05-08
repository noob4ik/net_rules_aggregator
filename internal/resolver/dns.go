package resolver

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

// ResolveResult holds resolved IPs for a single domain.
type ResolveResult struct {
	Domain string
	IPs    []netip.Addr
	Err    error
}

// ResolveDomain resolves A (and optionally AAAA) records for a domain.
// ipVersion: 4, 6, or 0 for both.
func ResolveDomain(ctx context.Context, domain string, ipVersion int) ResolveResult {
	res := ResolveResult{Domain: domain}

	resolver := net.DefaultResolver
	addrs, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		res.Err = fmt.Errorf("DNS lookup %q: %w", domain, err)
		return res
	}

	for _, a := range addrs {
		addr, ok := netip.AddrFromSlice(a.IP)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		switch ipVersion {
		case 4:
			if addr.Is4() {
				res.IPs = append(res.IPs, addr)
			}
		case 6:
			if addr.Is6() {
				res.IPs = append(res.IPs, addr)
			}
		default:
			res.IPs = append(res.IPs, addr)
		}
	}

	return res
}

// ResolveDomains resolves a list of domains concurrently with the given concurrency limit.
func ResolveDomains(ctx context.Context, domains []string, ipVersion int, concurrency int) []ResolveResult {
	if concurrency <= 0 {
		concurrency = 5
	}

	type job struct {
		domain string
	}

	jobs := make(chan job, len(domains))
	results := make(chan ResolveResult, len(domains))

	for i := 0; i < concurrency; i++ {
		go func() {
			for j := range jobs {
				results <- ResolveDomain(ctx, j.domain, ipVersion)
			}
		}()
	}

	for _, d := range domains {
		jobs <- job{domain: d}
	}
	close(jobs)

	out := make([]ResolveResult, 0, len(domains))
	for range domains {
		out = append(out, <-results)
	}
	return out
}
