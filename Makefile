.PHONY: test coverage lint clean mutate help

# Default target
.DEFAULT_GOAL := help

help:
	@echo "Trellis Go build targets:"
	@echo "  make test       - Run all tests"
	@echo "  make coverage   - Generate coverage report (coverage.html)"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make mutate     - Run mutation testing on core packages"
	@echo "  make clean      - Remove build artifacts and test outputs"
	@echo "  make build      - Build CLI binary to ./bin/trls"

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	golangci-lint run ./...

mutate:
	@echo "Running mutation tests on internal/dag..."
	gremlins unleash ./internal/dag

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/
	go clean -testcache

build:
	mkdir -p bin
	go build -o bin/trls ./cmd/trellis
