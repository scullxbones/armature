.PHONY: test coverage coverage-check lint adr-principles clean mutate check help skill dist-skills install build validate-skills validate-doc-examples deploy-skills trace-report skill-lint census-drift-check test-census-drift-check embed-examples

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
	@echo "  make check               - Run CI-safe validation: lint, test, coverage-check, mutate, validate-skills, validate-doc-examples, census-drift-check, build"
	@echo "  make test                - Run all tests"
	@echo "  make coverage            - Generate coverage report (coverage.html)"
	@echo "  make coverage-check      - Check coverage meets 80% threshold (fails build if not)"
	@echo "  make lint                - Run golangci-lint and ADR doc lint"
	@echo "  make mutate              - Run mutation testing on core packages"
	@echo "  make embed-examples      - Check that embedded skill examples match current CLI output (fails if drift detected)"
	@echo "  make validate-skills     - Validate embedded skill source (runs embed-examples check)"
	@echo "  make validate-doc-examples - Validate JSON examples in docs/skills against schemas"
	@echo "  make census-drift-check  - Verify code surfaces match docs/design/surface-census.md"
	@echo "  make test-census-drift-check - Test census-drift-check.sh itself (drift detection, both directions)"
	@echo "  make trace-report        - Scan test files for spec traceability patterns"
	@echo "  make clean               - Remove build artifacts and test outputs"
	@echo "  make build               - Build CLI binary to ./bin/arm"
	@echo "  make skill               - Build binary and deploy all skills/ to .claude/ and .gemini/ and .codex/"
	@echo "  make dist-skills         - Package skills for distribution (no binaries) into dist/"
	@echo "  make install             - Build binary and install to ~/.local/bin/arm (adds to PATH)"

check: lint build test coverage-check mutate validate-skills validate-doc-examples census-drift-check test-census-drift-check

trace-report:
	@$(PYTHON) scripts/trace_report.py .

test: build
	@tmp=$$(mktemp); \
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -json -count=1 ./... > "$$tmp"; status=$$?; \
	$(PYTHON) scripts/summarize_test_json.py "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status

coverage: build
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-check: build
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -coverprofile=coverage.out ./...
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | grep "^total:" | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if ! awk -v coverage="$${COVERAGE}" 'BEGIN { exit !(coverage >= 85) }'; then \
		echo "FAIL: coverage $${COVERAGE}% is below 85% threshold"; \
		exit 1; \
	fi

lint: adr-principles
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Then ensure ~/go/bin is on your PATH."; \
		exit 1; \
	}
	XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...

adr-principles:
	@$(PYTHON) scripts/check_adr_principles.py docs/adr

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
	rm -f "$$report"; \
	gremlins --config .gremlins.yaml --silent unleash --output "$$report" ./internal; status=$$?; \
	if [ -f "$$report" ]; then \
		$(PYTHON) scripts/summarize_gremlins_report.py "$$report" internal; \
	fi; \
	exit $$status
	@echo "Running mutation tests on cmd..."
	@report=mutesting-report/cmd.json; \
	rm -f "$$report"; \
	gremlins --config .gremlins.yaml --silent unleash --output "$$report" ./cmd; status=$$?; \
	if [ -f "$$report" ]; then \
		$(PYTHON) scripts/summarize_gremlins_report.py "$$report" cmd; \
	fi; \
	exit $$status

embed-examples: build
	@$(PYTHON) -m unittest scripts/test_embed_examples.py
	@ARM_BIN=$(CURDIR)/bin/arm $(PYTHON) scripts/embed_examples.py check

validate-skills: skill-lint embed-examples
	@if grep -rn "make install" internal/skillsembed/skills/*/SKILL.md 2>/dev/null; then \
		echo "FAIL: 'make install' found in skill bodies — remove it or replace with: 'If arm is not found, stop and resolve this before proceeding'"; \
		exit 1; \
	fi
	@echo "Skills validated: no 'make install' references and no example drift"

validate-doc-examples:
	@$(PYTHON) -c "import jsonschema" 2>/dev/null || $(PYTHON) -m pip install -q jsonschema
	@$(PYTHON) -m unittest scripts/test_validate_doc_examples.py
	@$(PYTHON) scripts/validate_doc_examples.py

skill-lint: build
	@ARM_BIN=$(CURDIR)/bin/arm $(PYTHON) scripts/skill_lint.py .

census-drift-check:
	@scripts/census-drift-check.sh .

test-census-drift-check:
	@scripts/test_census_drift_check.sh .

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/ .gemini/skills/
	$(GO) clean -testcache

build:
	mkdir -p bin
	GOFLAGS=-buildvcs=false CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/arm ./cmd/armature

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
