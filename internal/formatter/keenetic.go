package formatter

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"

	"net_rules_aggregator/internal/resolver"
)

// Keenetic writes routes in the format:
//
//	ROUTE ADD <network>      MASK <mask>   0.0.0.0 :: rem <ASN> <OrgName> [<source>]
func Keenetic(w io.Writer, entries []resolver.PrefixEntry) error {
	for _, e := range entries {
		if !e.CIDR.Addr().Is4() {
			continue // Keenetic format is IPv4-only
		}

		network := e.CIDR.Addr().String()
		mask := prefixLenToMask(e.CIDR)
		comment := buildComment(e)

		_, err := fmt.Fprintf(w, "ROUTE ADD %-18s MASK %-15s 0.0.0.0 :: rem %s\n",
			network, mask, comment)
		if err != nil {
			return err
		}
	}
	return nil
}

// prefixLenToMask converts a prefix length to dotted-decimal mask string.
func prefixLenToMask(pfx netip.Prefix) string {
	bits := pfx.Bits()
	mask := net.CIDRMask(bits, 32)
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

// buildComment builds the comment part of the Keenetic ROUTE line.
// Format: <ASN> <OrgName> [<source>]
func buildComment(e resolver.PrefixEntry) string {
	var parts []string
	if e.ASN != "" {
		parts = append(parts, e.ASN)
	}
	if e.Org != "" {
		parts = append(parts, e.Org)
	}
	if e.Source != "" {
		// Shorten source for readability: keep unique labels only.
		src := shortenSource(e.Source)
		if src != "" {
			parts = append(parts, "["+src+"]")
		}
	}
	if len(parts) == 0 {
		return e.CIDR.String()
	}
	return strings.Join(parts, " ")
}

// shortenSource trims redundant parts from source strings.
// e.g. "asn:AS13238,asn:AS13238" → "asn:AS13238"
func shortenSource(source string) string {
	seen := make(map[string]struct{})
	var parts []string
	for _, p := range strings.Split(source, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ",")
}
