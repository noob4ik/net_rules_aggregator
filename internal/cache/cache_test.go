package cache

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"net_rules_aggregator/internal/resolver"
)

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(s + ": " + err.Error())
	}
	return p.Masked()
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	entries := []resolver.PrefixEntry{
		{CIDR: mustPrefix("5.45.192.0/18"), ASN: "AS13238", Org: "Yandex LLC", Source: "asn:AS13238"},
		{CIDR: mustPrefix("77.88.0.0/18"), ASN: "AS13238", Org: "Yandex LLC", Source: "asn:AS13238"},
		{CIDR: mustPrefix("2001:db8::/32"), ASN: "AS1", Org: "Test Org", Source: "asn:AS1"},
	}
	asns := []string{"AS13238", "AS1"}

	path := filepath.Join(t.TempDir(), "cache.yaml")
	if err := Save(path, entries, asns); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(got) != len(entries) {
		t.Fatalf("len(got) = %d; want %d", len(got), len(entries))
	}
	for i, e := range entries {
		if got[i].CIDR != e.CIDR {
			t.Errorf("[%d] CIDR = %v; want %v", i, got[i].CIDR, e.CIDR)
		}
		if got[i].ASN != e.ASN {
			t.Errorf("[%d] ASN = %q; want %q", i, got[i].ASN, e.ASN)
		}
		if got[i].Org != e.Org {
			t.Errorf("[%d] Org = %q; want %q", i, got[i].Org, e.Org)
		}
		if got[i].Source != e.Source {
			t.Errorf("[%d] Source = %q; want %q", i, got[i].Source, e.Source)
		}
	}
}

func TestSave_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.yaml")
	if err := Save(path, nil, nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestLoad_InvalidCIDR(t *testing.T) {
	content := `generated_at: "2024-01-01T00:00:00Z"
prefixes:
  - cidr: "not_a_cidr"
    asn: AS1
`
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid CIDR in cache, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/file.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_HostBitsZeroed(t *testing.T) {
	// Cache entry with host bits set — Load must call Masked()
	content := `generated_at: "2024-01-01T00:00:00Z"
prefixes:
  - cidr: "10.0.0.1/8"
`
	path := filepath.Join(t.TempDir(), "masked.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := mustPrefix("10.0.0.0/8")
	if got[0].CIDR != want {
		t.Errorf("CIDR = %v; want %v (host bits should be zeroed)", got[0].CIDR, want)
	}
}

func TestSaveAndLoad_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yaml")
	if err := Save(path, nil, nil); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got))
	}
}
