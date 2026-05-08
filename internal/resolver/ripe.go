package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const ripeStatBase = "https://stat.ripe.net/data"

// KnownCDNASNs maps well-known CDN ASNs to their names for warning purposes.
var KnownCDNASNs = map[string]string{
	"AS13335": "Cloudflare",
	"AS20940": "Akamai",
	"AS16509": "Amazon CloudFront",
	"AS15169": "Google/YouTube CDN",
	"AS32934": "Meta/Facebook",
	"AS54113": "Fastly",
	"AS16625": "Akamai",
	"AS22822": "Limelight Networks",
	"AS30675": "Verizon Digital Media",
}

// ASNInfo holds an ASN number and the organisation name from RIPE.
type ASNInfo struct {
	ASN     string // e.g. "AS13238"
	OrgName string // e.g. "Yandex LLC"
}

// PrefixEntry is one prefix with its originating ASN info and source label.
type PrefixEntry struct {
	CIDR   netip.Prefix
	ASN    string
	Org    string
	Source string // e.g. "asn:AS13238" or "domain:youtube.com" or "ip:1.2.3.4"
}

// ripeClient is a thin HTTP client for RIPEstat.
type ripeClient struct {
	http    *http.Client
}

func newRIPEClient(timeout time.Duration) *ripeClient {
	return &ripeClient{
		http: &http.Client{Timeout: timeout},
	}
}

// IPToASN returns the ASN that originates the prefix containing addr.
func (c *ripeClient) IPToASN(ctx context.Context, addr netip.Addr) (ASNInfo, error) {
	url := fmt.Sprintf("%s/prefix-overview/data.json?resource=%s", ripeStatBase, addr.String())
	var resp struct {
		Data struct {
			ASNs []struct {
				ASN    int    `json:"asn"`
				Holder string `json:"holder"`
			} `json:"asns"`
		} `json:"data"`
	}
	if err := c.get(ctx, url, &resp); err != nil {
		return ASNInfo{}, fmt.Errorf("prefix-overview for %s: %w", addr, err)
	}
	if len(resp.Data.ASNs) == 0 {
		return ASNInfo{}, fmt.Errorf("no ASN found for IP %s", addr)
	}
	a := resp.Data.ASNs[0]
	return ASNInfo{
		ASN:     fmt.Sprintf("AS%d", a.ASN),
		OrgName: a.Holder,
	}, nil
}

// ASNOrgName returns the organisation name for a given ASN string like "AS13238".
func (c *ripeClient) ASNOrgName(ctx context.Context, asn string) (string, error) {
	num := strings.TrimPrefix(strings.ToUpper(asn), "AS")
	url := fmt.Sprintf("%s/as-overview/data.json?resource=AS%s", ripeStatBase, num)
	var resp struct {
		Data struct {
			Holder string `json:"holder"`
		} `json:"data"`
	}
	if err := c.get(ctx, url, &resp); err != nil {
		return "", fmt.Errorf("as-overview for %s: %w", asn, err)
	}
	return resp.Data.Holder, nil
}

// ASNPrefixes returns all announced prefixes for the given ASN.
func (c *ripeClient) ASNPrefixes(ctx context.Context, asn string, ipVersion int) ([]netip.Prefix, error) {
	num := strings.TrimPrefix(strings.ToUpper(asn), "AS")
	url := fmt.Sprintf("%s/announced-prefixes/data.json?resource=AS%s", ripeStatBase, num)
	var resp struct {
		Data struct {
			Prefixes []struct {
				Prefix string `json:"prefix"`
			} `json:"prefixes"`
		} `json:"data"`
	}
	if err := c.get(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("announced-prefixes for %s: %w", asn, err)
	}

	var out []netip.Prefix
	for _, p := range resp.Data.Prefixes {
		pfx, err := netip.ParsePrefix(p.Prefix)
		if err != nil {
			continue
		}
		pfx = pfx.Masked()
		switch ipVersion {
		case 4:
			if !pfx.Addr().Is4() {
				continue
			}
		case 6:
			if !pfx.Addr().Is6() || pfx.Addr().Is4In6() {
				continue
			}
		}
		out = append(out, pfx)
	}
	return out, nil
}

func (c *ripeClient) get(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "net_rules_aggregator/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return nil
}

// -------------------------------------------------------------------
// High-level resolver that orchestrates DNS + RIPE lookups
// -------------------------------------------------------------------

// Options controls the resolution process.
type Options struct {
	IPVersion   int           // 4, 6, or 0 (both)
	Concurrency int           // parallel RIPE API requests
	Timeout     time.Duration // per-request HTTP timeout
}

// ResolveAll takes validated input and returns a flat list of PrefixEntries.
// It writes CDN warnings to warnFn (called with a formatted message).
func ResolveAll(
	ctx context.Context,
	asns []string,
	domains []string,
	rawIPs []netip.Addr,
	directPrefixes []netip.Prefix,
	opts Options,
	warnFn func(string),
) ([]PrefixEntry, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 5
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}

	client := newRIPEClient(opts.Timeout)

	// Collect all ASNs with their source labels.
	type asnSource struct {
		asn    string
		source string
	}

	asnMap := make(map[string][]string) // asn → []sources

	addASN := func(asn, source string) {
		asn = strings.ToUpper(asn)
		if !strings.HasPrefix(asn, "AS") {
			asn = "AS" + asn
		}
		asnMap[asn] = append(asnMap[asn], source)
	}

	// 1. Direct ASNs from input.
	for _, asn := range asns {
		addASN(asn, "asn:"+asn)
	}

	// 2. Resolve domains → IPs → ASN.
	if len(domains) > 0 {
		dnsResults := ResolveDomains(ctx, domains, opts.IPVersion, opts.Concurrency)
		for _, r := range dnsResults {
			if r.Err != nil {
				warnFn(fmt.Sprintf("DNS error: %v", r.Err))
				continue
			}
			for _, ip := range r.IPs {
				info, err := client.IPToASN(ctx, ip)
				if err != nil {
					warnFn(fmt.Sprintf("RIPE lookup for %s (from domain %s): %v", ip, r.Domain, err))
					continue
				}
				addASN(info.ASN, fmt.Sprintf("domain:%s", r.Domain))
			}
		}
	}

	// 3. Resolve raw IPs → ASN.
	for _, ip := range rawIPs {
		info, err := client.IPToASN(ctx, ip)
		if err != nil {
			warnFn(fmt.Sprintf("RIPE lookup for IP %s: %v", ip, err))
			continue
		}
		addASN(info.ASN, fmt.Sprintf("ip:%s", ip))
	}

	// 4. Warn about CDN ASNs.
	for asn := range asnMap {
		if name, ok := KnownCDNASNs[asn]; ok {
			warnFn(fmt.Sprintf("WARNING: %s (%s) is a well-known CDN — adding all its prefixes may be undesirable", asn, name))
		}
	}

	// 5. Fetch prefixes for each unique ASN (with concurrency limit).
	type work struct {
		asn     string
		sources []string
	}

	jobs := make(chan work, len(asnMap))
	type result struct {
		asn      string
		org      string
		sources  []string
		prefixes []netip.Prefix
		err      error
	}
	results := make(chan result, len(asnMap))

	for i := 0; i < opts.Concurrency; i++ {
		go func() {
			for w := range jobs {
				org, err := client.ASNOrgName(ctx, w.asn)
				if err != nil {
					// Non-fatal: use empty org name.
					org = ""
				}
				pfxs, err := client.ASNPrefixes(ctx, w.asn, opts.IPVersion)
				results <- result{
					asn:      w.asn,
					org:      org,
					sources:  w.sources,
					prefixes: pfxs,
					err:      err,
				}
			}
		}()
	}

	for asn, sources := range asnMap {
		jobs <- work{asn: asn, sources: sources}
	}
	close(jobs)

	var entries []PrefixEntry
	for range asnMap {
		r := <-results
		if r.err != nil {
			warnFn(fmt.Sprintf("fetching prefixes for %s: %v", r.asn, r.err))
			continue
		}
		source := strings.Join(unique(r.sources), ",")
		for _, pfx := range r.prefixes {
			entries = append(entries, PrefixEntry{
				CIDR:   pfx,
				ASN:    r.asn,
				Org:    r.org,
				Source: source,
			})
		}
	}

	// 6. Include direct prefixes from input (no ASN lookup needed).
	for _, pfx := range directPrefixes {
		entries = append(entries, PrefixEntry{
			CIDR:   pfx,
			ASN:    "",
			Org:    "",
			Source: fmt.Sprintf("ip:%s", pfx),
		})
	}

	return entries, nil
}

func unique(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	var out []string
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
