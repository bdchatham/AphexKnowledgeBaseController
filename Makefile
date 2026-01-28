.PHONY: build lint test clean ci

# Build the controller binary
build:
	go build -o bin/knowledgebase-controller ./...

# Run linter
lint:
	golangci-lint run

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# CI target - runs lint and build
ci: lint build
