.PHONY: build run test clean dev

VERSION := 0.1.0
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)"

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o ifs-kiseki .

run: build
	./ifs-kiseki

test:
	CGO_ENABLED=1 go test ./... -v

clean:
	rm -f ifs-kiseki
	rm -f ifs-kiseki.db

dev:
	CGO_ENABLED=1 go run $(LDFLAGS) .
