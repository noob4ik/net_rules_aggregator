package formatter

import (
	"encoding/json"
	"io"

	"net_rules_aggregator/internal/resolver"
)

type amneziaOutput struct {
	Subnets []string `json:"subnets"`
}

// Amnezia writes the AmneziaVPN split-tunneling JSON format:
//
//	{"subnets": ["1.2.3.0/24", ...]}
func Amnezia(w io.Writer, entries []resolver.PrefixEntry) error {
	subnets := make([]string, 0, len(entries))
	for _, e := range entries {
		subnets = append(subnets, e.CIDR.String())
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(amneziaOutput{Subnets: subnets})
}
