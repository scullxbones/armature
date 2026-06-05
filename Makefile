.PHONY: test coverage coverage-check lint clean mutate check help skill dist-skills install build validate-skills deploy-skills

# Variables
GO ?= go
PYTHON ?= python3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -X main.Version=$(VERSION)
INSTALL_DIR ?= $(HOME)/.local/bin

# Default target
.DEFAULT_GOAL := help

help:
	@echo "Armature Go build targets:"
	@echo "  make check      - Run full CI validation: lint, test, coverage-check, mutate"
	@echo "  make test       - Run all tests"
	@echo "  make coverage   - Generate coverage report (coverage.html)"
	@echo "  make coverage-check - Check coverage meets 80% threshold (fails build if not)"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make mutate     - Run mutation testing on core packages"
	@echo "  make clean      - Remove build artifacts and test outputs"
	@echo "  make build      - Build CLI binary to ./bin/arm"
	@echo "  make skill      - Build binary and deploy all skills/ to .claude/ and .gemini/ and .codex/"
	@echo "  make dist-skills - Package skills for distribution (no binaries) into dist/"
	@echo "  make install    - Build binary and install to ~/.local/bin/arm (adds to PATH)"

check: lint test coverage-check mutate validate-skills skill

test:
	@tmp=$$(mktemp); \
	$(GO) test -json -count=1 ./... > "$$tmp"; status=$$?; \
	$(PYTHON) scripts/summarize_test_json.py "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-check:
	$(GO) test -coverprofile=coverage.out ./...
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
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
	XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...

mutate:
	@command -v gremlins >/dev/null 2>&1 || { \
		echo "gremlins not found. Install with:"; \
		echo "  go install github.com/go-gremlins/gremlins/cmd/gremlins@latest"; \
		echo "Then ensure ~/go/bin is on your PATH."; \
		exit 1; \
	}
	@echo "Running mutation tests on internal..."
	@mkdir -p mutesting-report
	@report=mutesting-report/internal.json; \
	gremlins --silent unleash --output "$$report" ./internal; status=$$?; \
	$(PYTHON) scripts/summarize_gremlins_report.py "$$report" internal; \
	exit $$status
	@echo "Running mutation tests on cmd..."
	@report=mutesting-report/cmd.json; \
	gremlins --silent unleash --output "$$report" ./cmd; status=$$?; \
	$(PYTHON) scripts/summarize_gremlins_report.py "$$report" cmd; \
	exit $$status

validate-skills:
	@if grep -rn "make install" internal/skillsembed/skills/*/SKILL.md 2>/dev/null; then \
		echo "FAIL: 'make install' found in skill bodies — remove it or replace with: 'If arm is not found, stop and resolve this before proceeding'"; \
		exit 1; \
	fi
	@echo "Skills validated: no 'make install' references"

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/ .gemini/skills/
	$(GO) clean -testcache

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/arm ./cmd/armature

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/arm $(INSTALL_DIR)/arm
	chmod +x $(INSTALL_DIR)/arm
	@echo "Installed arm to $(INSTALL_DIR)/arm"
	@echo "Ensure $(INSTALL_DIR) is on your PATH"

deploy-skills:
	@for name in internal/skillsembed/skills/*/; do \
		name=$$(basename "$$name"); \
		[ -f "internal/skillsembed/skills/$$name/SKILL.md" ] || continue; \
		for harness in claude gemini codex; do \
			mkdir -p ".$$harness/skills/$$name"; \
			cp -r "internal/skillsembed/skills/$$name/." ".$$harness/skills/$$name/"; \
		done; \
	done
	@echo "Deployed skills to .claude/skills/ and .gemini/skills/ and .codex/skills/"

skill: build deploy-skills

dist-skills:
	mkdir -p dist
	@for harness in claude gemini; do \
		$(PYTHON) -c "\
import zipfile, os, sys; \
harness = sys.argv[1]; \
base = '.'+harness+'/skills'; \
out = 'dist/skills-'+harness+'.zip'; \
zf = zipfile.ZipFile(out, 'w', zipfile.ZIP_DEFLATED); \
[ (zf.write(os.path.join(r,f), os.path.join(r,f)) \
   if 'scripts' not in r.split(os.sep) else None) \
  for r,_,fs in os.walk(base) for f in fs ]; \
zf.close(); \
print('Created '+out) \
" "$$harness"; \
	done
