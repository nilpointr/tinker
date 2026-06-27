.PHONY: build test vet lint clean

build:
	go build -o bin/tinker ./cmd/tinker

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	golangci-lint run ./...

clean:
	rm -rf bin/
