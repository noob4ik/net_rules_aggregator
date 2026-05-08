package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// --- unique ---

func TestUnique(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "no duplicates",
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "with duplicates",
			input: []string{"a", "b", "a", "c", "b"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "all same",
			input: []string{"x", "x", "x"},
			want:  []string{"x"},
		},
		{
			name:  "empty",
			input: []string{},
			want:  nil,
		},
		{
			name:  "nil",
			input: nil,
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unique(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("unique(%v) len = %d; want %d (got %v)", tc.input, len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("unique[%d] = %q; want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// --- ripeClient via httptest ---

// newTestServer returns an httptest.Server and a *ripeClient pointing at it.
func newTestServer(mux *http.ServeMux) (*httptest.Server, *ripeClient) {
	srv := httptest.NewServer(mux)
	client := &ripeClient{
		http: &http.Client{},
	}
	// Override the base URL by patching the client's http.Transport through closure
	// (we can't easily change ripeStatBase, so we use a custom round-tripper)
	client.http.Transport = &prefixRewriter{base: srv.URL, inner: http.DefaultTransport}
	return srv, client
}

// prefixRewriter rewrites requests to ripeStatBase → testServer URL.
type prefixRewriter struct {
	base  string
	inner http.RoundTripper
}

func (r *prefixRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the scheme+host with the test server URL
	newURL := r.base + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return r.inner.RoundTrip(newReq)
}

func TestIPToASN_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/prefix-overview/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"asns": []map[string]any{
					{"asn": 13238, "holder": "Yandex LLC"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	addr := netip.MustParseAddr("77.88.55.77")
	info, err := client.IPToASN(context.Background(), addr)
	if err != nil {
		t.Fatalf("IPToASN() error = %v", err)
	}
	if info.ASN != "AS13238" {
		t.Errorf("ASN = %q; want AS13238", info.ASN)
	}
	if info.OrgName != "Yandex LLC" {
		t.Errorf("OrgName = %q; want Yandex LLC", info.OrgName)
	}
}

func TestIPToASN_NoASN(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/prefix-overview/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"asns": []any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	addr := netip.MustParseAddr("1.2.3.4")
	_, err := client.IPToASN(context.Background(), addr)
	if err == nil {
		t.Fatal("expected error when no ASN found, got nil")
	}
}

func TestIPToASN_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/prefix-overview/data.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	addr := netip.MustParseAddr("1.2.3.4")
	_, err := client.IPToASN(context.Background(), addr)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestASNOrgName_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/as-overview/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"holder": "Yandex LLC",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	name, err := client.ASNOrgName(context.Background(), "AS13238")
	if err != nil {
		t.Fatalf("ASNOrgName() error = %v", err)
	}
	if name != "Yandex LLC" {
		t.Errorf("OrgName = %q; want Yandex LLC", name)
	}
}

func TestASNOrgName_NormalisesInput(t *testing.T) {
	// Should accept both "AS13238" and "13238"
	mux := http.NewServeMux()
	mux.HandleFunc("/data/as-overview/data.json", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("resource")
		// both should end up as "AS13238"
		if q != "AS13238" {
			http.Error(w, fmt.Sprintf("unexpected resource %q", q), http.StatusBadRequest)
			return
		}
		resp := map[string]any{"data": map[string]any{"holder": "Org"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	for _, asn := range []string{"AS13238", "as13238", "13238"} {
		_, err := client.ASNOrgName(context.Background(), asn)
		if err != nil {
			t.Errorf("ASNOrgName(%q) error = %v", asn, err)
		}
	}
}

func TestASNPrefixes_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/announced-prefixes/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"prefixes": []map[string]any{
					{"prefix": "5.45.192.0/18"},
					{"prefix": "77.88.0.0/18"},
					{"prefix": "2001:db8::/32"}, // IPv6 — should be filtered when ipVersion=4
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	// IPv4 only
	prefixes, err := client.ASNPrefixes(context.Background(), "AS13238", 4)
	if err != nil {
		t.Fatalf("ASNPrefixes() error = %v", err)
	}
	if len(prefixes) != 2 {
		t.Fatalf("len(prefixes) = %d; want 2 (IPv4 only)", len(prefixes))
	}
	for _, p := range prefixes {
		if !p.Addr().Is4() {
			t.Errorf("expected IPv4 prefix, got %v", p)
		}
	}
}

func TestASNPrefixes_IPv6Filter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/announced-prefixes/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"prefixes": []map[string]any{
					{"prefix": "5.45.192.0/18"},
					{"prefix": "2001:db8::/32"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	// IPv6 only
	prefixes, err := client.ASNPrefixes(context.Background(), "AS1", 6)
	if err != nil {
		t.Fatalf("ASNPrefixes() error = %v", err)
	}
	if len(prefixes) != 1 {
		t.Fatalf("len(prefixes) = %d; want 1 (IPv6 only)", len(prefixes))
	}
	if prefixes[0].Addr().Is4() {
		t.Errorf("expected IPv6 prefix, got %v", prefixes[0])
	}
}

func TestASNPrefixes_BothVersions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/announced-prefixes/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"prefixes": []map[string]any{
					{"prefix": "5.45.192.0/18"},
					{"prefix": "2001:db8::/32"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	// Both (ipVersion=0)
	prefixes, err := client.ASNPrefixes(context.Background(), "AS1", 0)
	if err != nil {
		t.Fatalf("ASNPrefixes() error = %v", err)
	}
	if len(prefixes) != 2 {
		t.Fatalf("len(prefixes) = %d; want 2 (both IPv4+IPv6)", len(prefixes))
	}
}

func TestASNPrefixes_SkipsInvalidPrefixes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/data/announced-prefixes/data.json", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"prefixes": []map[string]any{
					{"prefix": "not_a_prefix"},
					{"prefix": "10.0.0.0/24"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv, client := newTestServer(mux)
	defer srv.Close()

	prefixes, err := client.ASNPrefixes(context.Background(), "AS1", 4)
	if err != nil {
		t.Fatalf("ASNPrefixes() error = %v", err)
	}
	if len(prefixes) != 1 {
		t.Fatalf("len(prefixes) = %d; want 1 (invalid skipped)", len(prefixes))
	}
}

// --- ResolveDomains concurrency (no network, uses loopback which is available) ---

func TestResolveDomains_FallbackConcurrency(t *testing.T) {
	// concurrency <= 0 should default to 5 and not panic
	// We pass an empty domain list so no actual DNS calls are made
	results := ResolveDomains(context.Background(), nil, 4, 0)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

// --- KnownCDNASNs sanity check ---

func TestKnownCDNASNs_ContainsCloudflare(t *testing.T) {
	name, ok := KnownCDNASNs["AS13335"]
	if !ok {
		t.Fatal("AS13335 (Cloudflare) should be in KnownCDNASNs")
	}
	if name != "Cloudflare" {
		t.Errorf("KnownCDNASNs[AS13335] = %q; want Cloudflare", name)
	}
}
