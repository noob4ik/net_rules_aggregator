package formatter

import (
	"encoding/json"
	"io"

	"net_rules_aggregator/internal/resolver"
)

type amneziaEntry struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// Amnezia writes the AmneziaVPN split-tunneling JSON format:
//
//	[{"hostname": "1.2.3.0/24", "ip": ""}, ...]
func Amnezia(w io.Writer, entries []resolver.PrefixEntry) error {
	out := make([]amneziaEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, amneziaEntry{
			Hostname: e.CIDR.String(),
			IP:       "",
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
