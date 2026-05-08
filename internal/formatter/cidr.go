package formatter

import (
	"fmt"
	"io"

	"net_rules_aggregator/internal/resolver"
)

// CIDR writes one CIDR prefix per line.
func CIDR(w io.Writer, entries []resolver.PrefixEntry) error {
	for _, e := range entries {
		if _, err := fmt.Fprintln(w, e.CIDR.String()); err != nil {
			return err
		}
	}
	return nil
}
