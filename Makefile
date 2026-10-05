.PHONY: all build build-linux-arm64 build-linux-amd64 test lint clean docker run

BINARY_NAME=bin/harvester

all: test build

build:
	@mkdir -p bin
	go build -p 1 -ldflags="-s -w" -o $(BINARY_NAME) ./cmd/harvester

build-linux-arm64:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/harvester-linux-arm64 ./cmd/harvester

build-linux-amd64:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/harvester-linux-amd64 ./cmd/harvester

test:
	go test ./...

test-verbose:
	go test -v ./...

clean:
	rm -rf bin/ data/harvester.db* data/export/

docker:
	docker build -t viral-harvester:latest .

run: build
	./$(BINARY_NAME) -config configs/harvester.yaml
