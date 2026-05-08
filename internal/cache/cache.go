package cache

import (
	"fmt"
	"net/netip"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"net_rules_aggregator/internal/resolver"
)

// Entry is a serialisable form of resolver.PrefixEntry.
type Entry struct {
	CIDR   string `yaml:"cidr"`
	ASN    string `yaml:"asn,omitempty"`
	Org    string `yaml:"org,omitempty"`
	Source string `yaml:"source,omitempty"`
}

// Cache is the intermediate YAML file written between resolution and formatting.
type Cache struct {
	GeneratedAt string  `yaml:"generated_at"`
	SourceASNs  []string `yaml:"source_asns,omitempty"`
	Prefixes    []Entry  `yaml:"prefixes"`
}

// Save writes entries to path as YAML.
func Save(path string, entries []resolver.PrefixEntry, sourceASNs []string) error {
	c := Cache{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		SourceASNs:  sourceASNs,
		Prefixes:    make([]Entry, 0, len(entries)),
	}
	for _, e := range entries {
		c.Prefixes = append(c.Prefixes, Entry{
			CIDR:   e.CIDR.String(),
			ASN:    e.ASN,
			Org:    e.Org,
			Source: e.Source,
		})
	}

	data, err := yaml.Marshal(&c)
	if err != nil {
		return fmt.Errorf("marshalling cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing cache file %q: %w", path, err)
	}
	return nil
}

// Load reads a previously saved cache file and returns PrefixEntries.
func Load(path string) ([]resolver.PrefixEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cache file %q: %w", path, err)
	}

	var c Cache
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing cache YAML: %w", err)
	}

	entries := make([]resolver.PrefixEntry, 0, len(c.Prefixes))
	for _, e := range c.Prefixes {
		pfx, err := netip.ParsePrefix(e.CIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR in cache %q: %w", e.CIDR, err)
		}
		entries = append(entries, resolver.PrefixEntry{
			CIDR:   pfx.Masked(),
			ASN:    e.ASN,
			Org:    e.Org,
			Source: e.Source,
		})
	}
	return entries, nil
}
