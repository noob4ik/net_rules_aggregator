package aggregator

import (
	"net/netip"
	"sort"

	"net_rules_aggregator/internal/resolver"
)

// Aggregate takes a flat list of PrefixEntries, deduplicates them,
// and performs CIDR summarisation (merges adjacent subnets where possible).
// The per-prefix metadata (ASN, Org, Source) is preserved for the
// "representative" entry of each merged group (first seen wins).
func Aggregate(entries []resolver.PrefixEntry) []resolver.PrefixEntry {
	// 1. Deduplicate by CIDR string, keeping first metadata seen.
	seen := make(map[netip.Prefix]resolver.PrefixEntry, len(entries))
	order := make([]netip.Prefix, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.CIDR]; !ok {
			seen[e.CIDR] = e
			order = append(order, e.CIDR)
		}
	}

	// Separate IPv4 and IPv6 for independent aggregation.
	var v4, v6 []netip.Prefix
	for _, pfx := range order {
		if pfx.Addr().Is4() {
			v4 = append(v4, pfx)
		} else {
			v6 = append(v6, pfx)
		}
	}

	aggV4 := aggregatePrefixes(v4)
	aggV6 := aggregatePrefixes(v6)

	result := make([]resolver.PrefixEntry, 0, len(aggV4)+len(aggV6))
	for _, pfx := range aggV4 {
		result = append(result, bestEntry(pfx, seen, v4))
	}
	for _, pfx := range aggV6 {
		result = append(result, bestEntry(pfx, seen, v6))
	}
	return result
}

// bestEntry returns metadata for a (possibly merged) prefix.
// If the prefix exists verbatim in seen, use that entry.
// Otherwise use the first contained original prefix's metadata.
func bestEntry(merged netip.Prefix, seen map[netip.Prefix]resolver.PrefixEntry, originals []netip.Prefix) resolver.PrefixEntry {
	if e, ok := seen[merged]; ok {
		return e
	}
	// Find first original prefix contained in merged.
	for _, o := range originals {
		if merged.Contains(o.Addr()) && merged.Bits() <= o.Bits() {
			e := seen[o]
			e.CIDR = merged
			return e
		}
	}
	return resolver.PrefixEntry{CIDR: merged}
}

// aggregatePrefixes performs RFC-4632-style CIDR summarisation.
// It repeatedly tries to merge adjacent /N pairs into a /(N-1).
func aggregatePrefixes(prefixes []netip.Prefix) []netip.Prefix {
	if len(prefixes) == 0 {
		return nil
	}

	// Sort by address then prefix length (most specific last).
	sort.Slice(prefixes, func(i, j int) bool {
		return prefixLess(prefixes[i], prefixes[j])
	})

	// Remove prefixes that are already covered by a shorter one.
	prefixes = removeRedundant(prefixes)

	// Iteratively merge adjacent pairs.
	changed := true
	for changed {
		changed = false
		merged := make([]netip.Prefix, 0, len(prefixes))
		i := 0
		for i < len(prefixes) {
			if i+1 < len(prefixes) {
				if m, ok := tryMerge(prefixes[i], prefixes[i+1]); ok {
					merged = append(merged, m)
					i += 2
					changed = true
					continue
				}
			}
			merged = append(merged, prefixes[i])
			i++
		}
		prefixes = merged
		// Re-sort after merges.
		sort.Slice(prefixes, func(i, j int) bool {
			return prefixLess(prefixes[i], prefixes[j])
		})
		prefixes = removeRedundant(prefixes)
	}
	return prefixes
}

// tryMerge checks whether two same-length adjacent prefixes can be merged
// into their common parent.
func tryMerge(a, b netip.Prefix) (netip.Prefix, bool) {
	if a.Bits() != b.Bits() {
		return netip.Prefix{}, false
	}
	bits := a.Bits()
	if bits == 0 {
		return netip.Prefix{}, false
	}
	parentA := netip.PrefixFrom(a.Addr(), bits-1).Masked()
	parentB := netip.PrefixFrom(b.Addr(), bits-1).Masked()
	if parentA == parentB {
		return parentA, true
	}
	return netip.Prefix{}, false
}

// removeRedundant removes prefixes that are fully covered by a shorter prefix
// already present in the (sorted) slice.
func removeRedundant(sorted []netip.Prefix) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(sorted))
	for _, p := range sorted {
		redundant := false
		for _, super := range out {
			if super.Bits() < p.Bits() && super.Overlaps(p) {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, p)
		}
	}
	return out
}

func prefixLess(a, b netip.Prefix) bool {
	if a.Addr() != b.Addr() {
		ab, _ := a.Addr().MarshalBinary()
		bb, _ := b.Addr().MarshalBinary()
		for i := range ab {
			if i >= len(bb) {
				return false
			}
			if ab[i] != bb[i] {
				return ab[i] < bb[i]
			}
		}
		return len(ab) < len(bb)
	}
	return a.Bits() < b.Bits()
}
