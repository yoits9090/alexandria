.PHONY: test run build fmt
VERSION ?= dev
fmt:
	gofmt -w cmd internal
test:
	go test ./...
build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/alexandria ./cmd/alexandria
run:
	go run ./cmd/alexandria
