# net_rules_aggregator — Agent Instructions

CLI utility that generates subnet lists from ASNs, domains, and IPs,
formatted for import into Keenetic (static routes) and AmneziaVPN (split tunneling).

## Project structure

```
cmd/main.go                      # CLI entry point (cobra), all flags wired here
internal/input/loader.go         # Parses and validates input YAML
internal/resolver/dns.go         # Concurrent DNS resolution: domain → []IP
internal/resolver/ripe.go        # RIPEstat API: IP→ASN, ASN→prefixes+org name
internal/aggregator/aggregator.go# Deduplication + CIDR summarisation
internal/cache/cache.go          # Read/write intermediate YAML cache
internal/formatter/keenetic.go   # ROUTE ADD <net> MASK <mask> 0.0.0.0 :: rem ...
internal/formatter/amnezia.go    # [{"hostname":"","ip":"...","comment":"..."}, ...]
internal/formatter/cidr.go       # Plain CIDR list, one per line
internal/formatter/yaml.go       # Structured YAML output
input.example.yaml               # Example input file (copy to input.yaml and edit)
```

## Key types

- `input.Validated` — parsed input: ASNs, Domains, RawIPs, Prefixes
- `resolver.PrefixEntry` — a single prefix with CIDR, ASN, Org, Source fields
- `resolver.Options` — IPVersion (4/6/0), Concurrency, Timeout
- `cache.Cache` — serialisable form of the intermediate result

## Pipeline

```
input.Load(path)
    └─► resolver.ResolveAll(asns, domains, rawIPs, directPrefixes, opts, warnFn)
            ├─ ResolveDomains (DNS, concurrent)
            ├─ ripeClient.IPToASN   (RIPEstat prefix-overview)
            ├─ ripeClient.ASNOrgName (RIPEstat as-overview)
            └─ ripeClient.ASNPrefixes (RIPEstat announced-prefixes)
    └─► aggregator.Aggregate(entries)   — dedup + CIDR summarisation
    └─► cache.Save(path, entries, asns) — write cache.yaml
    └─► formatter.{Keenetic,Amnezia,CIDR,YAML}(w, entries)
```

## CLI flags

| Flag | Default | Notes |
|------|---------|-------|
| `-i, --input` | `input.yaml` | Input YAML |
| `-f, --format` | `cidr` | keenetic / amnezia / cidr / yaml |
| `-o, --output` | stdout | Output file |
| `--cache-file` | `cache.yaml` | Intermediate cache |
| `--skip-resolve` | false | Load cache, skip network |
| `--ip-version` | `4` | 4 / 6 / both |
| `--concurrency` | 5 | Parallel RIPE API requests |
| `--timeout` | 30s | HTTP timeout |

## Coding conventions

- All packages under `internal/` — nothing is exported outside the module.
- Public types/functions use Go doc comments.
- Errors are wrapped with `fmt.Errorf("context: %w", err)`.
- Warnings (CDN ASNs, DNS failures, RIPE errors) go to stderr via `warnFn` callback — never `log.Fatal`.
- No global state; all config is passed explicitly via function arguments.
- `aggregator.Aggregate` must handle both IPv4 and IPv6 independently.
- CIDR aggregation: sort → remove redundant (covered by shorter prefix) → merge adjacent pairs iteratively.

## Data sources

- DNS: `net.DefaultResolver` (system resolver)
- IP → ASN: `https://stat.ripe.net/data/prefix-overview/data.json?resource=<IP>`
- ASN → prefixes: `https://stat.ripe.net/data/announced-prefixes/data.json?resource=<ASN>`
- ASN → org name: `https://stat.ripe.net/data/as-overview/data.json?resource=<ASN>`

## Known CDN ASNs (warn, don't block)

AS13335 Cloudflare, AS20940 Akamai, AS16509 Amazon CloudFront,
AS15169 Google/YouTube CDN, AS32934 Meta/Facebook, AS54113 Fastly.
Defined in `resolver.KnownCDNASNs`.

## Build & run

```bash
go build ./...           # compile check
go vet ./...             # static analysis
go run ./cmd/main.go --help
cp input.example.yaml input.yaml   # create your own input from the example
go run ./cmd/main.go -i input.yaml -f keenetic
go run ./cmd/main.go --skip-resolve -f amnezia
```

## Testing

```bash
go test ./...            # run all tests
go test -v ./...         # verbose output
go test ./internal/aggregator/...   # single package
go test -run TestAggregate ./...    # single test by name
```

Test files live next to the package they test (`*_test.go`):

| File | Package | What is covered |
|------|---------|-----------------|
| `internal/input/loader_test.go` | `input` | `NormaliseASN` edge cases, `Load` (valid/invalid YAML, host-bit zeroing, blank skipping, IPv6 prefixes) |
| `internal/aggregator/aggregator_test.go` | `aggregator` | `prefixLess`, `tryMerge`, `removeRedundant`, `aggregatePrefixes`, `Aggregate` (dedup, merge, metadata preservation, IPv4/IPv6 independence) |
| `internal/formatter/formatter_test.go` | `formatter` | `CIDR`, `Amnezia`, `Keenetic` (IPv6 filtering, CIDR fallback comment), `YAML`, `prefixLenToMask`, `buildComment`, `shortenSource` |
| `internal/cache/cache_test.go` | `cache` | `Save`+`Load` round-trip, host-bit zeroing on load, invalid CIDR error, missing file error |
| `internal/resolver/resolver_test.go` | `resolver` | `unique`, `IPToASN`, `ASNOrgName`, `ASNPrefixes` (IP version filtering, invalid prefix skipping), `ResolveDomains` concurrency fallback — all HTTP calls mocked via `httptest.Server` |

**Conventions:**
- Tests do not make real network calls; RIPE API responses are served by `httptest.Server` with a custom `http.RoundTripper` that redirects requests.
- Table-driven tests (`t.Run`) are preferred for functions with many input variants.
- Temporary files use `t.TempDir()` — cleaned up automatically.
- DNS tests only use empty domain lists to avoid real lookups.

## Output format examples

**keenetic**
```
ROUTE ADD 5.45.192.0       MASK 255.255.192.0   0.0.0.0 :: rem AS13238 Yandex LLC [asn:AS13238]
```

**amnezia**
```json
[
 {"hostname": "5.45.192.0/18", "ip": ""},
 {"hostname": "77.88.0.0/18", "ip": ""}
]
```

**cidr**
```
5.45.192.0/18
77.88.0.0/18
```
