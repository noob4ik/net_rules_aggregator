package formatter

import (
	"io"
	"time"

	"gopkg.in/yaml.v3"

	"net_rules_aggregator/internal/resolver"
)

type yamlEntry struct {
	CIDR   string `yaml:"cidr"`
	ASN    string `yaml:"asn,omitempty"`
	Org    string `yaml:"org,omitempty"`
	Source string `yaml:"source,omitempty"`
}

type yamlOutput struct {
	GeneratedAt string      `yaml:"generated_at"`
	Count       int         `yaml:"count"`
	Prefixes    []yamlEntry `yaml:"prefixes"`
}

// YAML writes a structured YAML document with all prefix entries.
func YAML(w io.Writer, entries []resolver.PrefixEntry) error {
	out := yamlOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Count:       len(entries),
		Prefixes:    make([]yamlEntry, 0, len(entries)),
	}
	for _, e := range entries {
		out.Prefixes = append(out.Prefixes, yamlEntry{
			CIDR:   e.CIDR.String(),
			ASN:    e.ASN,
			Org:    e.Org,
			Source: e.Source,
		})
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(out)
}
