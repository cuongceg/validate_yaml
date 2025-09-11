.PHONY: build test fmt lint example-config run proto

build:
	go build ./...

test:
	go test ./...

fmt:
	go fmt ./...

proto:
	protoc --go_out=./proto/ ./proto/envelop/envelop.proto

example-config:
	go run ./cmd/cfgcheck --config configs/user_config.yaml --example true

run:
	go run ./cmd/cfgcheck --config configs/user_config.yaml
