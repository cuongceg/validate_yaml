BIN := bin/cfgcheck

DEFAULT: run
.PHONY: build run lint test clean

test:
	go test -v ./internal/config/

build: test
	@mkdir -p bin
	GO111MODULE=on go build -o $(BIN) ./cmd/cfgcheck

run: build
	./$(BIN) -c ./configs/config.example.yaml --print

clean:
	rm -rf bin