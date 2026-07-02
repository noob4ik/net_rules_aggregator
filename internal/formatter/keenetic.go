package formatter

import (
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
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

// KeeneticPagePath returns the file path for a given page number (1-based).
// Format: <basePath>_<NNN>.txt where NNN is zero-padded to three digits.
func KeeneticPagePath(basePath string, page int) string {
	return fmt.Sprintf("%s_%03d.txt", basePath, page)
}

// writeKeeneticPage creates the file at path, writes entries in Keenetic
// format, and closes it. Both write and close errors are propagated; if a
// write error occurs the close error is not silently dropped.
func writeKeeneticPage(path string, entries []resolver.PrefixEntry) (retErr error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating page file %q: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("closing page file %q: %w", path, cerr)
		}
	}()
	if err := Keenetic(f, entries); err != nil {
		return fmt.Errorf("writing page to %q: %w", path, err)
	}
	return nil
}

// KeeneticPaged splits IPv4 entries into pages of pageSize routes and writes
// each page into a separate file named <basePath>_001.txt, _002.txt, etc.
// It returns the list of created file paths.
// On any error, an empty slice is returned together with the error; the caller
// cannot rely on partial results.
// basePath must be a writable file path without extension; the extension
// ".txt" and a zero-padded three-digit page number are appended automatically.
func KeeneticPaged(basePath string, entries []resolver.PrefixEntry, pageSize int) ([]string, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("pageSize must be positive, got %d", pageSize)
	}

	// Collect only IPv4 entries.
	var ipv4 []resolver.PrefixEntry
	for _, e := range entries {
		if e.CIDR.Addr().Is4() {
			ipv4 = append(ipv4, e)
		}
	}

	if len(ipv4) == 0 {
		return nil, nil
	}

	pages := (len(ipv4) + pageSize - 1) / pageSize
	paths := make([]string, 0, pages)

	for p := range pages {
		start := p * pageSize
		end := start + pageSize
		if end > len(ipv4) {
			end = len(ipv4)
		}

		path := KeeneticPagePath(basePath, p+1)
		if err := writeKeeneticPage(path, ipv4[start:end]); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}

	return paths, nil
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
