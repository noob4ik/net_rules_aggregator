.PHONY: build test keenetic amnezia clean

ARTIFACTS := cache.yaml \
	keenetic_routes.bat \
	routes.txt \
	routes \
	result.yaml \
	amnezia_sites.json \
	amnezia_sites_*.json \
	routes*.txt \
	routes*.yaml

KEENETIC_PAGE_SIZE ?= 1000

clean:
	rm -f $(ARTIFACTS)

build:
	go build ./...

test:
	go test ./...

keenetic:
	go run ./cmd/main.go -f keenetic --page-size $(KEENETIC_PAGE_SIZE) -o routes

amnezia:
	go run ./cmd/main.go -f amnezia -o amnezia_sites.json
