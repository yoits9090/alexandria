.PHONY: test run build fmt
fmt:
	gofmt -w cmd internal
test:
	go test ./...
build:
	go build -o bin/alexandria ./cmd/alexandria
run:
	go run ./cmd/alexandria
