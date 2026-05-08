# net_rules_aggregator

CLI utility for generating subnet lists from ASNs, domains, and IPs — formatted for import into **Keenetic** static routes and **AmneziaVPN** split tunneling.

## How it works

```
Input YAML (ASNs, domains, IPs)
        │
        ▼
[1] DNS resolution: domain → IP
        │
        ▼
[2] RIPEstat API: IP → ASN, ASN → prefix list + org name
        │
        ▼
[3] CIDR aggregation (merge adjacent subnets)
        │
        ▼
[4] Save intermediate cache (cache.yaml)
        │
        ▼
[5] Format output: keenetic | amnezia | cidr | yaml
```

## Installation

```bash
git clone https://github.com/yourname/net_rules_aggregator
cd net_rules_aggregator
go build -o net_rules_aggregator ./cmd/main.go
```

Requires Go 1.23+.

## Input YAML

```yaml
asn:
  - AS13238   # Yandex
  - AS47764   # Mail.ru

domains:
  - vk.com
  - ok.ru

ips:
  - 77.88.55.77       # single IP → resolved to ASN automatically
  - 5.45.192.0/18     # direct prefix → included as-is
```

## Usage

```bash
# Full pipeline: resolve + aggregate + output
net_rules_aggregator -i input.yaml -f keenetic -o routes.txt

# Use cached result (no network requests)
net_rules_aggregator --skip-resolve -f amnezia -o amnezia_sites.json

# Include IPv6 prefixes
net_rules_aggregator -i input.yaml --ip-version 6 -f cidr

# Both IPv4 and IPv6, YAML output
net_rules_aggregator -i input.yaml --ip-version both -f yaml -o result.yaml
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-i, --input` | `input.yaml` | Input YAML file |
| `-f, --format` | `cidr` | Output format: `keenetic`, `amnezia`, `cidr`, `yaml` |
| `-o, --output` | stdout | Output file path |
| `--cache-file` | `cache.yaml` | Intermediate resolution cache |
| `--skip-resolve` | `false` | Skip resolution, use existing cache |
| `--ip-version` | `4` | IP version: `4`, `6`, or `both` |
| `--concurrency` | `5` | Parallel RIPE API requests |
| `--timeout` | `30s` | HTTP request timeout |

## Output formats

### `keenetic` — Keenetic static routes

```
ROUTE ADD 5.45.192.0       MASK 255.255.192.0   0.0.0.0 :: rem AS13238 Yandex LLC [asn:AS13238]
ROUTE ADD 77.88.0.0        MASK 255.255.192.0   0.0.0.0 :: rem AS13238 Yandex LLC [ip:77.88.55.77]
ROUTE ADD 217.69.128.0     MASK 255.255.192.0   0.0.0.0 :: rem AS47764 Mail.Ru LLC [domain:mail.ru]
```

### `amnezia` — AmneziaVPN split tunneling

```bash
net_rules_aggregator -i input.yaml -f amnezia -o amnezia_sites.json
```

```json
[
  {
    "hostname": "",
    "ip": "5.45.192.0/18",
    "comment": "AS13238 Yandex LLC [asn:AS13238]"
  },
  {
    "hostname": "",
    "ip": "77.88.0.0/18",
    "comment": "AS13238 Yandex LLC [ip:77.88.55.77]"
  },
  {
    "hostname": "",
    "ip": "217.69.128.0/18",
    "comment": "AS47764 Mail.Ru LLC [domain:mail.ru]"
  }
]
```

### `cidr` — plain CIDR list

```
5.45.192.0/18
77.88.0.0/18
217.69.128.0/18
```

### `yaml` — structured YAML

```yaml
generated_at: "2026-05-08T12:00:00Z"
count: 3
prefixes:
  - cidr: 5.45.192.0/18
    asn: AS13238
    org: Yandex LLC
    source: asn:AS13238
```

## Cache file

After resolution, results are saved to `cache.yaml` (configurable via `--cache-file`):

```yaml
generated_at: "2026-05-08T12:00:00Z"
source_asns:
  - AS13238
  - AS47764
prefixes:
  - cidr: 5.45.192.0/18
    asn: AS13238
    org: Yandex LLC
    source: asn:AS13238
```

Use `--skip-resolve` to reformat without hitting the network again.

## CDN warnings

When a resolved domain or IP belongs to a well-known CDN (Cloudflare, Akamai, Google, Meta, Fastly, Amazon CloudFront), a warning is printed to stderr:

```
[WARN] WARNING: AS13335 (Cloudflare) is a well-known CDN — adding all its prefixes may be undesirable
```

## Data sources

- **DNS resolution** — system resolver (`net.DefaultResolver`)
- **IP → ASN** — [RIPEstat prefix-overview API](https://stat.ripe.net/docs/02.data-api/prefix-overview.html)
- **ASN → prefixes** — [RIPEstat announced-prefixes API](https://stat.ripe.net/docs/02.data-api/announced-prefixes.html)
- **ASN org name** — [RIPEstat as-overview API](https://stat.ripe.net/docs/02.data-api/as-overview.html)

## License

[MIT](LICENSE)
