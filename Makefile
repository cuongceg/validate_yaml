.PHONY: build test fmt lint example-config run

build:
	go build ./...

test:
	go test ./...

fmt:
	go fmt ./...

example-config:
	go run ./cmd/cfgcheck --config configs/user_configs.yaml --example true

run:
	go run ./cmd/cfgcheck --config configs/user_configs.yaml
