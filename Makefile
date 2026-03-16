.PHONY: test coverage lint clean mutate help skill

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
	@echo "  make skill      - Build binary and deploy trls AgentSkill to .claude/skills/trls/"

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
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/
	go clean -testcache

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/trls ./cmd/trellis

skill: build
	mkdir -p .claude/skills/trls/scripts
	cat docs/trls-skill-meta.yaml docs/SKILL.md > .claude/skills/trls/SKILL.md
	cp bin/trls .claude/skills/trls/scripts/trls
	chmod +x .claude/skills/trls/scripts/trls
