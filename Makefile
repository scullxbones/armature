.PHONY: test test-skill-transcript test-e2eharness coverage coverage-check lint adr-principles clean mutate check check-fast test-check-fast help skill dist-skills install build validate-skills validate-doc-examples deploy-skills trace-report skill-lint census-drift-check test-census-drift-check embed-examples crosscompile

# Variables
GO ?= go
PYTHON ?= python3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -X main.Version=$(VERSION)
INSTALL_DIR ?= $(HOME)/.local/bin
UNIT_PACKAGES := $(shell GOCACHE=$${GOCACHE:-/tmp/armature-gocache} GOFLAGS=$${GOFLAGS:--buildvcs=false} $(GO) list ./... | grep -v '/internal/e2eharness$$')

# Default target
.DEFAULT_GOAL := help

help:
	@echo "Armature Go build targets:"
	@echo "  make check               - Run CI-safe validation: lint, build, coverage-check, mutate, validate-skills, validate-doc-examples, census-drift-check, crosscompile"
	@echo "  make check-fast          - Diff-routed fast gate: only runs steps implied by changed files (BASE= to override diff base)"
	@echo "  make test-check-fast     - Test check-fast.sh routing itself"
	@echo "  make test                - Run unit tests (E2E harness has a dedicated target)"
	@echo "  make test-skill-transcript - Run coordinator skill golden transcript tests"
	@echo "  make test-e2eharness     - Run full end-to-end harness suite (separate CI job)"
	@echo "  make coverage            - Generate coverage report (coverage.html)"
	@echo "  make coverage-check      - Run coverage then fail if cmd < 83% or internal < 86%"
	@echo "  make lint                - Run golangci-lint and ADR doc lint"
	@echo "  make mutate              - Run mutation testing on core packages"
	@echo "  make embed-examples      - Check that embedded skill examples match current CLI output (fails if drift detected)"
	@echo "  make validate-skills     - Validate embedded skills and canonical CLI documentation"
	@echo "  make validate-doc-examples - Validate JSON examples in docs/skills against schemas"
	@echo "  make census-drift-check  - Verify code surfaces match docs/design/surface-census.md"
	@echo "  make test-census-drift-check - Test census-drift-check.sh itself (drift detection, both directions)"
	@echo "  make trace-report        - Scan test files for spec traceability patterns"
	@echo "  make clean               - Remove build artifacts and test outputs"
	@echo "  make build               - Build CLI binary to ./bin/arm"
	@echo "  make crosscompile        - Build (no test) every platform .goreleaser.yaml ships, to catch platform-specific compile breakage"
	@echo "  make skill               - Build binary and deploy all skills/ to .claude/ and .gemini/ and .codex/"
	@echo "  make dist-skills         - Package skills for distribution (no binaries) into dist/"
	@echo "  make install             - Build binary and install to ~/.local/bin/arm (adds to PATH)"

check: lint build coverage-check mutate validate-skills validate-doc-examples census-drift-check test-census-drift-check crosscompile

trace-report:
	@$(PYTHON) scripts/trace_report.py .

test: build
	@tmp=$$(mktemp); \
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -json -count=1 $(UNIT_PACKAGES) > "$$tmp"; status=$$?; \
	$(PYTHON) scripts/summarize_test_json.py "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status

test-skill-transcript: build
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -v -count=1 ./internal/skilltranscript/...

test-e2eharness: build
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -v -count=1 ./internal/e2eharness/...

coverage: build
	@tmp=$$(mktemp); \
	ARM_BIN=$(CURDIR)/bin/arm $(GO) test -json -count=1 -coverprofile=coverage.out $(UNIT_PACKAGES) > "$$tmp"; status=$$?; \
	$(PYTHON) scripts/summarize_test_json.py "$$tmp"; \
	rm -f "$$tmp"; \
	exit $$status
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Reads the coverage.out profile produced by `coverage` rather than re-running
# the unit suite (D3, docs/design/gate-efficiency.md): the full gate runs the
# suite exactly once. Declared as a prerequisite so `make -j check` cannot
# start this target against a stale or missing profile.
coverage-check: coverage
	@if [ ! -f coverage.out ]; then \
		echo "FAIL: coverage.out not found; run 'make coverage' first"; \
		exit 1; \
	fi
	@awk 'NR>1{n=$$2;c=$$3; \
		if($$0 ~ /armature\/cmd\//){ct+=n; if(c>0) cc+=n} \
		if($$0 ~ /armature\/internal\//){it+=n; if(c>0) ic+=n}} \
	END{ \
		cmd_pct = (ct>0) ? 100*cc/ct : 0; \
		int_pct = (it>0) ? 100*ic/it : 0; \
		printf "cmd coverage: %.2f%%\n", cmd_pct; \
		printf "internal coverage: %.2f%%\n", int_pct; \
		fail=0; \
		if (cmd_pct < 83) { printf "FAIL: cmd coverage %.2f%% is below 83%% threshold (short by %.2f points)\n", cmd_pct, 83-cmd_pct; fail=1 } \
		if (int_pct < 86) { printf "FAIL: internal coverage %.2f%% is below 86%% threshold (short by %.2f points)\n", int_pct, 86-int_pct; fail=1 } \
		if (ct==0) { print "FAIL: no coverage lines matched armature/cmd/ — tree missing from profile"; fail=1 } \
		if (it==0) { print "FAIL: no coverage lines matched armature/internal/ — tree missing from profile"; fail=1 } \
		exit fail \
	}' coverage.out

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
	@echo "Skills and canonical documentation validated: no 'make install' references and no example drift"

validate-doc-examples:
	@go run ./cmd/armature validate doc-examples --repo .

skill-lint: build
	@ARM_BIN=$(CURDIR)/bin/arm $(PYTHON) scripts/skill_lint.py .

census-drift-check:
	@scripts/census-drift-check.sh .

test-census-drift-check:
	@scripts/test_census_drift_check.sh .

check-fast:
	@scripts/check-fast.sh .

test-check-fast:
	@scripts/test-check-fast.sh .

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/ .gemini/skills/
	$(GO) clean -testcache

build:
	mkdir -p bin
	GOFLAGS=-buildvcs=false CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/arm ./cmd/armature

# Platform list mirrors .goreleaser.yaml's builds.goos/goarch/ignore exactly:
# linux+darwin get amd64/arm64; windows ships amd64 only (goreleaser ignores
# windows/arm64). Build-only (no tests) so this stays fast enough for `check`;
# it exists to catch platform-specific compile breakage like undefined
# syscall constants on non-unix platforms before it ships silently broken.
crosscompile:
	@for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "Cross-compiling $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch GOFLAGS=-buildvcs=false CGO_ENABLED=0 $(GO) build -o /dev/null ./... || exit 1; \
	done
	@echo "Cross-compile check passed for all shipped platforms"

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
