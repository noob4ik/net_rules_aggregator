package input

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Input represents the parsed input YAML file.
type Input struct {
	ASN     []string `yaml:"asn"`
	Domains []string `yaml:"domains"`
	IPs     []string `yaml:"ips"`
}

// Validated holds normalised entries after parsing.
type Validated struct {
	ASNs     []string       // e.g. "AS13238"
	Domains  []string       // e.g. "youtube.com"
	Prefixes []netip.Prefix // parsed CIDR prefixes from the ips field
	RawIPs   []netip.Addr   // single IPs from the ips field (no prefix length)
}

// Load reads and validates the input YAML file at the given path.
func Load(path string) (*Validated, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading input file: %w", err)
	}

	var raw Input
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing input YAML: %w", err)
	}

	v := &Validated{}

	for _, asn := range raw.ASN {
		norm := NormaliseASN(asn)
		if norm == "" {
			return nil, fmt.Errorf("invalid ASN value: %q", asn)
		}
		v.ASNs = append(v.ASNs, norm)
	}

	for _, d := range raw.Domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		v.Domains = append(v.Domains, d)
	}

	for _, ip := range raw.IPs {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if strings.Contains(ip, "/") {
			pfx, err := netip.ParsePrefix(ip)
			if err != nil {
				return nil, fmt.Errorf("invalid prefix in ips: %q: %w", ip, err)
			}
			v.Prefixes = append(v.Prefixes, pfx.Masked())
		} else {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return nil, fmt.Errorf("invalid IP in ips: %q: %w", ip, err)
			}
			v.RawIPs = append(v.RawIPs, addr)
		}
	}

	return v, nil
}

// NormaliseASN accepts "AS13238", "as13238", "13238" and returns "AS13238".
func NormaliseASN(s string) string {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "AS") {
		num := upper[2:]
		if num == "" {
			return ""
		}
		return "AS" + num
	}
	if s == "" {
		return ""
	}
	return "AS" + s
}
