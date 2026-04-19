VERSION ?= 1.0.0
BUILD_TIME := $(shell date -u +%Y%m%d%H%M%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)
DISGUISE_NAME := enterprise-sync

.PHONY: all build-linux build-darwin build-windows clean test

all: build-linux build-darwin build-windows

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/$(DISGUISE_NAME) cmd/phantom/main.go

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/$(DISGUISE_NAME)-darwin cmd/phantom/main.go

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/$(DISGUISE_NAME).exe cmd/phantom/main.go

test:
	go test ./pkg/... -v -count=1

clean:
	rm -rf bin/
