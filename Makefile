.PHONY: test coverage coverage-check lint clean mutate check help skill install

# Default target
.DEFAULT_GOAL := help

help:
	@echo "Trellis Go build targets:"
	@echo "  make check      - Run full CI validation: lint, test, coverage-check, mutate"
	@echo "  make test       - Run all tests"
	@echo "  make coverage   - Generate coverage report (coverage.html)"
	@echo "  make coverage-check - Check coverage meets 80% threshold (fails build if not)"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make mutate     - Run mutation testing on core packages"
	@echo "  make clean      - Remove build artifacts and test outputs"
	@echo "  make build      - Build CLI binary to ./bin/trls"
	@echo "  make skill      - Build binary and deploy trls AgentSkill to .claude/skills/trls/"
	@echo "  make install    - Build binary and install to ~/.local/bin/trls (adds to PATH)"

check: lint test coverage-check mutate skill

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-check:
	go test -coverprofile=coverage.out ./...
	@COVERAGE=$$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ $$(echo "$${COVERAGE} < 80" | bc -l) -eq 1 ]; then \
		echo "FAIL: coverage $${COVERAGE}% is below 80% threshold"; \
		exit 1; \
	fi

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Then ensure ~/go/bin is on your PATH."; \
		exit 1; \
	}
	golangci-lint run ./...

mutate:
	@command -v gremlins >/dev/null 2>&1 || { \
		echo "gremlins not found. Install with:"; \
		echo "  go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"; \
		echo "Then ensure ~/go/bin is on your PATH."; \
		exit 1; \
	}
	@echo "Running mutation tests on internal..."
	gremlins unleash ./internal
	@echo "Running mutation tests on cmd..."
	gremlins unleash ./cmd

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/ .gemini/skills/
	go clean -testcache

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/trls ./cmd/trellis

install: build
	mkdir -p ~/.local/bin
	cp bin/trls ~/.local/bin/trls
	chmod +x ~/.local/bin/trls
	@echo "Installed trls to ~/.local/bin/trls"
	@echo "Ensure ~/.local/bin is on your PATH"

skill: build
	mkdir -p .claude/skills/trls/scripts
	cat skills/trls/meta.yaml skills/trls/SKILL.md > .claude/skills/trls/SKILL.md
	cp bin/trls .claude/skills/trls/scripts/trls
	chmod +x .claude/skills/trls/scripts/trls
	mkdir -p .claude/skills/trls-worker
	{ cat skills/trls-worker/meta.yaml; printf '> **DO NOT EDIT** — generated from `skills/trls-worker/SKILL.md` via `make skill`. Edit the source file and re-run `make skill`.\n\n'; cat skills/trls-worker/SKILL.md; } > .claude/skills/trls-worker/SKILL.md
	mkdir -p .gemini/skills/trls/scripts
	cat skills/trls/meta.yaml skills/trls/SKILL.md > .gemini/skills/trls/SKILL.md
	cp bin/trls .gemini/skills/trls/scripts/trls
	chmod +x .gemini/skills/trls/scripts/trls
	mkdir -p .gemini/skills/trls-worker
	{ cat skills/trls-worker/meta.yaml; printf '> **DO NOT EDIT** — generated from `skills/trls-worker/SKILL.md` via `make skill`. Edit the source file and re-run `make skill`.\n\n'; cat skills/trls-worker/SKILL.md; } > .gemini/skills/trls-worker/SKILL.md
	@echo "Deployed trls and trls-worker skills to .claude/skills/ and .gemini/skills/"
