package formatter

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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

// --- CIDR formatter ---

func TestCIDR_Basic(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entry("10.0.0.0/8"),
		entry("192.168.1.0/24"),
	}
	var buf bytes.Buffer
	if err := CIDR(&buf, entries); err != nil {
		t.Fatalf("CIDR() error = %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d; want 2", len(lines))
	}
	if lines[0] != "10.0.0.0/8" {
		t.Errorf("lines[0] = %q; want 10.0.0.0/8", lines[0])
	}
	if lines[1] != "192.168.1.0/24" {
		t.Errorf("lines[1] = %q; want 192.168.1.0/24", lines[1])
	}
}

func TestCIDR_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := CIDR(&buf, nil); err != nil {
		t.Fatalf("CIDR() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestCIDR_IPv6(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entry("2001:db8::/32"),
	}
	var buf bytes.Buffer
	if err := CIDR(&buf, entries); err != nil {
		t.Fatalf("CIDR() error = %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "2001:db8::/32" {
		t.Errorf("CIDR output = %q; want 2001:db8::/32", got)
	}
}

// --- Amnezia formatter ---

type amneziaEntryTest struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

func TestAmnezia_Basic(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entryFull("5.45.192.0/18", "AS13238", "Yandex LLC", "asn:AS13238"),
		entry("77.88.0.0/18"),
	}
	var buf bytes.Buffer
	if err := Amnezia(&buf, entries); err != nil {
		t.Fatalf("Amnezia() error = %v", err)
	}

	var out []amneziaEntryTest
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal error = %v; output: %q", err, buf.String())
	}
	if len(out) != 2 {
		t.Fatalf("len = %d; want 2", len(out))
	}
	if out[0].Hostname != "5.45.192.0/18" {
		t.Errorf("out[0].hostname = %q; want 5.45.192.0/18", out[0].Hostname)
	}
	if out[0].IP != "" {
		t.Errorf("out[0].ip = %q; want empty string", out[0].IP)
	}
	if out[1].Hostname != "77.88.0.0/18" {
		t.Errorf("out[1].hostname = %q; want 77.88.0.0/18", out[1].Hostname)
	}
	if out[1].IP != "" {
		t.Errorf("out[1].ip = %q; want empty string", out[1].IP)
	}
}

func TestAmnezia_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := Amnezia(&buf, nil); err != nil {
		t.Fatalf("Amnezia() error = %v", err)
	}
	var out []amneziaEntryTest
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal error = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("output should be empty array, got %v", out)
	}
}

func TestAmnezia_IncludesIPv6(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entry("10.0.0.0/24"),
		entry("2001:db8::/32"),
	}
	var buf bytes.Buffer
	if err := Amnezia(&buf, entries); err != nil {
		t.Fatalf("Amnezia() error = %v", err)
	}
	var out []amneziaEntryTest
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON unmarshal error = %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 entries (IPv4+IPv6), got %d: %v", len(out), out)
	}
}

// --- Keenetic formatter ---

func TestKeenetic_Basic(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entryFull("5.45.192.0/18", "AS13238", "Yandex LLC", "asn:AS13238"),
	}
	var buf bytes.Buffer
	if err := Keenetic(&buf, entries); err != nil {
		t.Fatalf("Keenetic() error = %v", err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	if !strings.HasPrefix(line, "ROUTE ADD ") {
		t.Errorf("expected line to start with ROUTE ADD, got %q", line)
	}
	if !strings.Contains(line, "5.45.192.0") {
		t.Errorf("expected IP in line, got %q", line)
	}
	if !strings.Contains(line, "255.255.192.0") {
		t.Errorf("expected mask 255.255.192.0 in line, got %q", line)
	}
	if !strings.Contains(line, "0.0.0.0") {
		t.Errorf("expected gateway 0.0.0.0 in line, got %q", line)
	}
	if !strings.Contains(line, "AS13238") {
		t.Errorf("expected ASN in comment, got %q", line)
	}
	if !strings.Contains(line, "Yandex LLC") {
		t.Errorf("expected org name in comment, got %q", line)
	}
}

func TestKeenetic_SkipsIPv6(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entry("2001:db8::/32"),
		entryFull("10.0.0.0/24", "AS1", "OrgA", "asn:AS1"),
	}
	var buf bytes.Buffer
	if err := Keenetic(&buf, entries); err != nil {
		t.Fatalf("Keenetic() error = %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// Only 1 line — IPv6 entry should be skipped
	if len(lines) != 1 {
		t.Errorf("expected 1 line (IPv4 only), got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "10.0.0.0") {
		t.Errorf("expected 10.0.0.0 in output, got %q", lines[0])
	}
}

func TestKeenetic_NoMetadata_FallbackToCIDR(t *testing.T) {
	entries := []resolver.PrefixEntry{
		{CIDR: mustPrefix("192.168.0.0/24")},
	}
	var buf bytes.Buffer
	if err := Keenetic(&buf, entries); err != nil {
		t.Fatalf("Keenetic() error = %v", err)
	}
	line := buf.String()
	if !strings.Contains(line, "192.168.0.0/24") {
		t.Errorf("expected CIDR fallback in comment, got %q", line)
	}
}

func TestKeenetic_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := Keenetic(&buf, nil); err != nil {
		t.Fatalf("Keenetic() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

// --- prefixLenToMask ---

func TestPrefixLenToMask(t *testing.T) {
	tests := []struct {
		cidr string
		mask string
	}{
		{"0.0.0.0/0", "0.0.0.0"},
		{"0.0.0.0/8", "255.0.0.0"},
		{"0.0.0.0/16", "255.255.0.0"},
		{"0.0.0.0/24", "255.255.255.0"},
		{"0.0.0.0/32", "255.255.255.255"},
		{"5.45.192.0/18", "255.255.192.0"},
	}
	for _, tc := range tests {
		t.Run(tc.cidr, func(t *testing.T) {
			pfx := mustPrefix(tc.cidr)
			got := prefixLenToMask(pfx)
			if got != tc.mask {
				t.Errorf("prefixLenToMask(%v) = %q; want %q", pfx, got, tc.mask)
			}
		})
	}
}

// --- buildComment ---

func TestBuildComment_AllFields(t *testing.T) {
	e := entryFull("1.0.0.0/8", "AS1", "Org1", "asn:AS1")
	got := buildComment(e)
	if !strings.Contains(got, "AS1") {
		t.Errorf("expected ASN in comment, got %q", got)
	}
	if !strings.Contains(got, "Org1") {
		t.Errorf("expected org in comment, got %q", got)
	}
	if !strings.Contains(got, "[asn:AS1]") {
		t.Errorf("expected source in comment, got %q", got)
	}
}

func TestBuildComment_NoFields_FallbackToCIDR(t *testing.T) {
	e := resolver.PrefixEntry{CIDR: mustPrefix("10.0.0.0/8")}
	got := buildComment(e)
	if got != "10.0.0.0/8" {
		t.Errorf("buildComment = %q; want 10.0.0.0/8", got)
	}
}

// --- shortenSource ---

func TestShortenSource_Deduplication(t *testing.T) {
	got := shortenSource("asn:AS1,asn:AS1,asn:AS2")
	want := "asn:AS1,asn:AS2"
	if got != want {
		t.Errorf("shortenSource = %q; want %q", got, want)
	}
}

func TestShortenSource_NoDuplicates(t *testing.T) {
	got := shortenSource("asn:AS1,domain:foo.com")
	want := "asn:AS1,domain:foo.com"
	if got != want {
		t.Errorf("shortenSource = %q; want %q", got, want)
	}
}

func TestShortenSource_Empty(t *testing.T) {
	got := shortenSource("")
	if got != "" {
		t.Errorf("shortenSource(\"\") = %q; want \"\"", got)
	}
}

// --- YAML formatter ---

func TestYAML_Basic(t *testing.T) {
	entries := []resolver.PrefixEntry{
		entryFull("10.0.0.0/8", "AS1", "Org1", "asn:AS1"),
		entryFull("192.168.0.0/16", "AS2", "Org2", "asn:AS2"),
	}
	var buf bytes.Buffer
	if err := YAML(&buf, entries); err != nil {
		t.Fatalf("YAML() error = %v", err)
	}

	var out struct {
		GeneratedAt string `yaml:"generated_at"`
		Count       int    `yaml:"count"`
		Prefixes    []struct {
			CIDR   string `yaml:"cidr"`
			ASN    string `yaml:"asn"`
			Org    string `yaml:"org"`
			Source string `yaml:"source"`
		} `yaml:"prefixes"`
	}
	if err := yaml.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("YAML unmarshal error = %v; output: %q", err, buf.String())
	}

	if out.Count != 2 {
		t.Errorf("count = %d; want 2", out.Count)
	}
	if len(out.Prefixes) != 2 {
		t.Fatalf("len(prefixes) = %d; want 2", len(out.Prefixes))
	}
	if out.Prefixes[0].CIDR != "10.0.0.0/8" {
		t.Errorf("prefixes[0].cidr = %q; want 10.0.0.0/8", out.Prefixes[0].CIDR)
	}
	if out.Prefixes[0].ASN != "AS1" {
		t.Errorf("prefixes[0].asn = %q; want AS1", out.Prefixes[0].ASN)
	}
	if out.GeneratedAt == "" {
		t.Errorf("generated_at should not be empty")
	}
}

func TestYAML_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := YAML(&buf, nil); err != nil {
		t.Fatalf("YAML() error = %v", err)
	}
	var out struct {
		Count    int    `yaml:"count"`
		Prefixes []any  `yaml:"prefixes"`
	}
	if err := yaml.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("YAML unmarshal error = %v", err)
	}
	if out.Count != 0 {
		t.Errorf("count = %d; want 0", out.Count)
	}
}
