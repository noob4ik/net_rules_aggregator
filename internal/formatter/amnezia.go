package formatter

import (
	"encoding/json"
	"io"

	"net_rules_aggregator/internal/resolver"
)

type amneziaEntry struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Comment  string `json:"comment,omitempty"`
}

// Amnezia writes the AmneziaVPN split-tunneling JSON format:
//
//	[{"hostname": "", "ip": "1.2.3.0/24", "comment": "..."}, ...]
func Amnezia(w io.Writer, entries []resolver.PrefixEntry) error {
	out := make([]amneziaEntry, 0, len(entries))
	for _, e := range entries {
		item := amneziaEntry{
			Hostname: "",
			IP:       e.CIDR.String(),
			Comment:  buildComment(e),
		}
		// If comment equals the CIDR itself (fallback), omit it to reduce noise
		// when there is no meaningful metadata.
		if item.Comment == item.IP {
			item.Comment = ""
		}
		out = append(out, item)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
