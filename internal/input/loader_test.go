package input

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// --- NormaliseASN ---

func TestNormaliseASN(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"uppercase with prefix", "AS13238", "AS13238"},
		{"lowercase with prefix", "as13238", "AS13238"},
		{"mixed case with prefix", "As13238", "AS13238"},
		{"numeric only", "13238", "AS13238"},
		{"numeric with spaces", "  13238  ", "AS13238"},
		{"AS with spaces", "  AS13238  ", "AS13238"},
		{"empty string", "", ""},
		{"only spaces", "   ", ""},
		{"AS prefix no number", "AS", ""},
		{"as prefix no number", "as", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormaliseASN(tc.input)
			if got != tc.want {
				t.Errorf("NormaliseASN(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- Load ---

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "input.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestLoad_BasicValid(t *testing.T) {
	yaml := `
asn:
  - AS13238
  - 47764
domains:
  - example.com
  - vk.com
ips:
  - 77.88.55.77
  - 5.45.192.0/18
`
	path := writeTemp(t, yaml)
	v, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(v.ASNs) != 2 {
		t.Errorf("len(ASNs) = %d; want 2", len(v.ASNs))
	}
	if v.ASNs[0] != "AS13238" {
		t.Errorf("ASNs[0] = %q; want AS13238", v.ASNs[0])
	}
	if v.ASNs[1] != "AS47764" {
		t.Errorf("ASNs[1] = %q; want AS47764", v.ASNs[1])
	}

	if len(v.Domains) != 2 {
		t.Errorf("len(Domains) = %d; want 2", len(v.Domains))
	}

	if len(v.RawIPs) != 1 {
		t.Errorf("len(RawIPs) = %d; want 1", len(v.RawIPs))
	}
	if v.RawIPs[0].String() != "77.88.55.77" {
		t.Errorf("RawIPs[0] = %q; want 77.88.55.77", v.RawIPs[0])
	}

	if len(v.Prefixes) != 1 {
		t.Errorf("len(Prefixes) = %d; want 1", len(v.Prefixes))
	}
	if v.Prefixes[0].String() != "5.45.192.0/18" {
		t.Errorf("Prefixes[0] = %q; want 5.45.192.0/18", v.Prefixes[0])
	}
}

func TestLoad_PrefixHostBitsZeroed(t *testing.T) {
	// 5.45.255.1/18 has host bits set — Load must zero them → 5.45.192.0/18
	yaml := `
ips:
  - 5.45.255.1/18
`
	path := writeTemp(t, yaml)
	v, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := netip.MustParsePrefix("5.45.192.0/18")
	if v.Prefixes[0] != want {
		t.Errorf("Prefixes[0] = %v; want %v", v.Prefixes[0], want)
	}
}

func TestLoad_SkipsBlankDomainAndIP(t *testing.T) {
	yaml := `
domains:
  - ""
  - "   "
  - example.com
ips:
  - ""
  - "   "
  - 1.2.3.4
`
	path := writeTemp(t, yaml)
	v, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(v.Domains) != 1 || v.Domains[0] != "example.com" {
		t.Errorf("Domains = %v; want [example.com]", v.Domains)
	}
	if len(v.RawIPs) != 1 {
		t.Errorf("RawIPs = %v; want 1 entry", v.RawIPs)
	}
}

func TestLoad_InvalidASN(t *testing.T) {
	yaml := `
asn:
  - ""
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty ASN, got nil")
	}
}

func TestLoad_InvalidPrefix(t *testing.T) {
	yaml := `
ips:
  - not_a_prefix/24
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid prefix, got nil")
	}
}

func TestLoad_InvalidIP(t *testing.T) {
	yaml := `
ips:
  - not_an_ip
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid IP, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/file.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	path := writeTemp(t, "")
	v, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(v.ASNs) != 0 || len(v.Domains) != 0 || len(v.RawIPs) != 0 || len(v.Prefixes) != 0 {
		t.Errorf("expected empty Validated, got %+v", v)
	}
}

func TestLoad_IPv6Prefix(t *testing.T) {
	yaml := `
ips:
  - 2001:db8::/32
`
	path := writeTemp(t, yaml)
	v, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(v.Prefixes) != 1 {
		t.Fatalf("len(Prefixes) = %d; want 1", len(v.Prefixes))
	}
	if v.Prefixes[0].String() != "2001:db8::/32" {
		t.Errorf("Prefixes[0] = %q; want 2001:db8::/32", v.Prefixes[0])
	}
}
