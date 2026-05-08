package aggregator

import (
	"net/netip"
	"testing"

	"net_rules_aggregator/internal/resolver"
)

// helpers

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(s + ": " + err.Error())
	}
	return p.Masked()
}

func entry(cidr string) resolver.PrefixEntry {
	return resolver.PrefixEntry{CIDR: mustPrefix(cidr)}
}

func entryFull(cidr, asn, org, src string) resolver.PrefixEntry {
	return resolver.PrefixEntry{
		CIDR:   mustPrefix(cidr),
		ASN:    asn,
		Org:    org,
		Source: src,
	}
}

// --- prefixLess ---

func TestPrefixLess(t *testing.T) {
	// 1.0.0.0/8 < 2.0.0.0/8 (by address)
	a := mustPrefix("1.0.0.0/8")
	b := mustPrefix("2.0.0.0/8")
	if !prefixLess(a, b) {
		t.Errorf("expected 1.0.0.0/8 < 2.0.0.0/8")
	}
	if prefixLess(b, a) {
		t.Errorf("expected 2.0.0.0/8 NOT < 1.0.0.0/8")
	}

	// same addr, different bits: shorter is smaller
	c := mustPrefix("10.0.0.0/8")
	d := mustPrefix("10.0.0.0/16")
	if !prefixLess(c, d) {
		t.Errorf("expected /8 < /16")
	}
	if prefixLess(d, c) {
		t.Errorf("expected /16 NOT < /8")
	}

	// equal
	if prefixLess(a, a) {
		t.Errorf("expected equal NOT less")
	}
}

// --- tryMerge ---

func TestTryMerge_Success(t *testing.T) {
	// 10.0.0.0/25 + 10.0.0.128/25 → 10.0.0.0/24
	a := mustPrefix("10.0.0.0/25")
	b := mustPrefix("10.0.0.128/25")
	got, ok := tryMerge(a, b)
	if !ok {
		t.Fatal("tryMerge should succeed")
	}
	want := mustPrefix("10.0.0.0/24")
	if got != want {
		t.Errorf("tryMerge = %v; want %v", got, want)
	}
}

func TestTryMerge_DifferentLengths(t *testing.T) {
	a := mustPrefix("10.0.0.0/24")
	b := mustPrefix("10.0.1.0/25")
	_, ok := tryMerge(a, b)
	if ok {
		t.Error("tryMerge should fail for different prefix lengths")
	}
}

func TestTryMerge_NonAdjacent(t *testing.T) {
	// 10.0.0.0/24 and 10.0.2.0/24 are not adjacent siblings
	a := mustPrefix("10.0.0.0/24")
	b := mustPrefix("10.0.2.0/24")
	_, ok := tryMerge(a, b)
	if ok {
		t.Error("tryMerge should fail for non-adjacent prefixes")
	}
}

func TestTryMerge_ZeroBits(t *testing.T) {
	// /0 prefixes cannot be merged
	a := netip.PrefixFrom(netip.MustParseAddr("0.0.0.0"), 0)
	b := netip.PrefixFrom(netip.MustParseAddr("128.0.0.0"), 0)
	_, ok := tryMerge(a, b)
	if ok {
		t.Error("tryMerge should fail for /0 prefixes")
	}
}

// --- removeRedundant ---

func TestRemoveRedundant(t *testing.T) {
	// 10.0.0.0/8 covers 10.0.0.0/24 and 10.1.0.0/16 — they should be removed
	sorted := []netip.Prefix{
		mustPrefix("10.0.0.0/8"),
		mustPrefix("10.0.0.0/24"),
		mustPrefix("10.1.0.0/16"),
		mustPrefix("192.168.0.0/16"),
	}
	got := removeRedundant(sorted)
	if len(got) != 2 {
		t.Fatalf("removeRedundant len = %d; want 2, got %v", len(got), got)
	}
	if got[0] != sorted[0] {
		t.Errorf("got[0] = %v; want %v", got[0], sorted[0])
	}
	if got[1] != sorted[3] {
		t.Errorf("got[1] = %v; want %v", got[1], sorted[3])
	}
}

func TestRemoveRedundant_NoRedundant(t *testing.T) {
	prefixes := []netip.Prefix{
		mustPrefix("10.0.0.0/24"),
		mustPrefix("10.0.1.0/24"),
	}
	got := removeRedundant(prefixes)
	if len(got) != 2 {
		t.Errorf("removeRedundant len = %d; want 2", len(got))
	}
}

// --- aggregatePrefixes ---

func TestAggregatePrefixes_MergeAdjacentPair(t *testing.T) {
	// Two adjacent /25 → one /24
	prefixes := []netip.Prefix{
		mustPrefix("10.0.0.128/25"),
		mustPrefix("10.0.0.0/25"),
	}
	got := aggregatePrefixes(prefixes)
	want := []netip.Prefix{mustPrefix("10.0.0.0/24")}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("aggregatePrefixes = %v; want %v", got, want)
	}
}

func TestAggregatePrefixes_MergeChain(t *testing.T) {
	// Four /26 → /24
	prefixes := []netip.Prefix{
		mustPrefix("10.0.0.0/26"),
		mustPrefix("10.0.0.64/26"),
		mustPrefix("10.0.0.128/26"),
		mustPrefix("10.0.0.192/26"),
	}
	got := aggregatePrefixes(prefixes)
	want := mustPrefix("10.0.0.0/24")
	if len(got) != 1 || got[0] != want {
		t.Errorf("aggregatePrefixes = %v; want [%v]", got, want)
	}
}

func TestAggregatePrefixes_RemovesSubsumed(t *testing.T) {
	// /16 already covers /24
	prefixes := []netip.Prefix{
		mustPrefix("10.0.0.0/16"),
		mustPrefix("10.0.0.0/24"),
	}
	got := aggregatePrefixes(prefixes)
	if len(got) != 1 || got[0] != mustPrefix("10.0.0.0/16") {
		t.Errorf("aggregatePrefixes = %v; want [10.0.0.0/16]", got)
	}
}

func TestAggregatePrefixes_Empty(t *testing.T) {
	got := aggregatePrefixes(nil)
	if got != nil {
		t.Errorf("aggregatePrefixes(nil) = %v; want nil", got)
	}
}

func TestAggregatePrefixes_SinglePrefix(t *testing.T) {
	p := mustPrefix("192.168.1.0/24")
	got := aggregatePrefixes([]netip.Prefix{p})
	if len(got) != 1 || got[0] != p {
		t.Errorf("aggregatePrefixes([1]) = %v; want [%v]", got, p)
	}
}

// --- Aggregate (public, with metadata) ---

func TestAggregate_DeduplicatesEntries(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entryFull("10.0.0.0/24", "AS1", "Org1", "asn:AS1"),
		entryFull("10.0.0.0/24", "AS2", "Org2", "asn:AS2"), // duplicate
	}
	got := Aggregate(entries)
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1", len(got))
	}
	// First-seen metadata should win
	if got[0].ASN != "AS1" {
		t.Errorf("ASN = %q; want AS1", got[0].ASN)
	}
}

func TestAggregate_MergesAndPreservesMetadata(t *testing.T) {
	// Two adjacent /25 should merge to /24; metadata from the first contained prefix
	entries := []resolver.PrefixEntry{
		entryFull("10.0.0.0/25", "AS10", "OrgA", "asn:AS10"),
		entryFull("10.0.0.128/25", "AS20", "OrgB", "asn:AS20"),
	}
	got := Aggregate(entries)
	if len(got) != 1 {
		t.Fatalf("len = %d; want 1 (merged /24)", len(got))
	}
	if got[0].CIDR.String() != "10.0.0.0/24" {
		t.Errorf("CIDR = %v; want 10.0.0.0/24", got[0].CIDR)
	}
}

func TestAggregate_IPv4andIPv6Independent(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entry("10.0.0.0/25"),
		entry("10.0.0.128/25"),
		entry("2001:db8::/33"),
		entry("2001:db8:8000::/33"),
	}
	got := Aggregate(entries)
	// Should yield 10.0.0.0/24 and 2001:db8::/32
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2, got %v", len(got), got)
	}

	cidrStrs := make(map[string]bool)
	for _, e := range got {
		cidrStrs[e.CIDR.String()] = true
	}
	if !cidrStrs["10.0.0.0/24"] {
		t.Errorf("missing 10.0.0.0/24 in %v", got)
	}
	if !cidrStrs["2001:db8::/32"] {
		t.Errorf("missing 2001:db8::/32 in %v", got)
	}
}

func TestAggregate_Empty(t *testing.T) {
	got := Aggregate(nil)
	if len(got) != 0 {
		t.Errorf("Aggregate(nil) = %v; want []", got)
	}
}
