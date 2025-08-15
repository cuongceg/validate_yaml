.PHONY: build test fmt lint

build:
	go build ./...

test:
	go test ./...

fmt:
	go fmt ./...

run:
	go run ./cmd/cfgcheck --config configs/config.example.yaml --strict --print-default=false --format=json
