.PHONY: test test-ci test-skill-transcript test-e2eharness coverage coverage-check test-coverage-check lint adr-principles clean mutate check check-job check-git-floor print-check-jobs print-govulncheck-version print-tool-versions install-golangci-lint-release install-gremlins-release install-rg-release install-syft-release test-install-verified-release install-check-tools test-check-jobs check-fast test-check-fast help skill dist-skills install build validate-skills validate-doc-examples validate-graph deploy-skills trace-report skill-lint census-drift-check test-census-drift-check git-test-hermetic-check test-git-test-hermetic-check git-test-exec-check test-git-test-exec-check embed-examples crosscompile context-report test-summarize-test-json commit-subjects-check mock-ban-check test-mock-ban source-manifest-decode-check test-source-manifest-decode surface-budget-check surface-budget-ratchet test-surface-budget-check

GO ?= go
PYTHON ?= python3
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -X main.Version=$(VERSION)
INSTALL_DIR ?= $(HOME)/.local/bin
GOOS_NATIVE := $(shell $(GO) env GOOS)
ifeq ($(GOOS_NATIVE),windows)
ARM_EXE := .exe
else
ARM_EXE :=
endif
ARM_BIN := $(CURDIR)/bin/arm$(ARM_EXE)
UNIT_PACKAGES := $(shell GOCACHE=$${GOCACHE:-$${TMPDIR:-/tmp}/armature-gocache} GOFLAGS=$${GOFLAGS:--buildvcs=false} $(GO) list ./... | grep -v '/internal/e2e/harness$$' | grep -v '/scripts/surfacebudget$$' | grep -v '/scripts/repoguard$$')

.DEFAULT_GOAL := help

help:
	@echo "Armature Go build targets:"
	@echo "  make check               - Run publish gate serially (union of check-static, check-core, mutate)"
	@echo "  make check-job JOB=<id>  - Run one CI partition (check-static | check-core | mutate)"
	@echo "  make check-git-floor     - Full CHECK_JOBS suite without commit-subjects (CI exact-Git-floor leg)"
	@echo "  make print-check-jobs    - Dump the check-job membership table"
	@echo "  make install-check-tools - go install pinned golangci-lint and gremlins (local-dev)"
	@echo "  make check-fast          - Diff-routed fast gate: commit subjects plus steps implied by changed files (BASE= to override diff base)"
	@echo "  make commit-subjects-check - Fail unscoped conventional-commit subjects on origin/main..HEAD"
	@echo "  make test-check-fast     - Test check-fast.sh routing itself"
	@echo "  make test                - Run unit tests (E2E harness has a dedicated target)"
	@echo "  make test-ci             - OS-matrix test entry: full make test on unix; go test -c on windows"
	@echo "  make test-summarize-test-json - Test go test -json timing summarizer"
	@echo "  make test-skill-transcript - Run coordinator skill golden transcript tests"
	@echo "  make test-e2eharness     - Run full end-to-end harness suite (separate CI job)"
	@echo "  make coverage            - Generate coverage report (coverage.html)"
	@echo "  make coverage-check      - Run coverage then fail if cmd < 83% or internal < 86%"
	@echo "  make test-coverage-check - Test coverage-check.sh threshold logic itself (pass/fail fixtures)"
	@echo "  make lint                - Run golangci-lint and ADR doc lint"
	@echo "  make mutate              - Run mutation testing on core packages"
	@echo "  make embed-examples      - Check that embedded skill examples match current CLI output (fails if drift detected)"
	@echo "  make validate-skills     - Validate embedded and .agents/skills plus canonical CLI documentation"
	@echo "  make validate-doc-examples - Validate JSON examples in docs/skills against schemas"
	@echo "  make validate-graph      - arm validate --ci (story integration / CI; not part of make check)"
	@echo "  make census-drift-check  - Verify code surfaces match docs/design/surface-census.md"
	@echo "  make test-census-drift-check - Test census-drift-check.sh itself (drift detection, both directions)"
	@echo "  make mock-ban-check      - Fail if go.mod lists a mock framework or mockery output is present"
	@echo "  make test-mock-ban       - Test check-mock-ban.sh and the depguard mock-ban rule"
	@echo "  make source-manifest-decode-check - Fail if a second sources/manifest.json decoder is introduced"
	@echo "  make test-source-manifest-decode - Test the typed source-manifest repo guard"
	@echo "  make surface-budget-check - Fail if an internal package grew its exported identifier count"
	@echo "  make surface-budget-ratchet - Rewrite the committed internal export baseline (shrinks, or reviewed growth)"
	@echo "  make test-surface-budget-check - Test the surface-budget ratchet (growth fail, shrink pass, rewrite)"
	@echo "  make git-test-hermetic-check - Fail if _test.go files use raw git init outside internal/gittest"
	@echo "  make test-git-test-hermetic-check - Test the typed git-hermetic repo guard"
	@echo "  make git-test-exec-check - Fail if _test.go writes an executable outside testexec.Write"
	@echo "  make test-git-test-exec-check - Test the typed test-executables repo guard"
	@echo "  make trace-report        - Scan test files for spec traceability patterns"
	@echo "  make context-report      - Price fixture-measured main-path CLI payloads (bytes and bytes/4 tokens)"
	@echo "  make clean               - Remove build artifacts and test outputs"
	@echo "  make build               - Build CLI binary to ./bin/arm"
	@echo "  make crosscompile        - Build (no test) every platform .goreleaser.yaml ships, to catch platform-specific compile breakage"
	@echo "  make skill               - Build binary and deploy all skills/ to .claude/ and .gemini/ and .codex/"
	@echo "  make dist-skills         - Package skills for distribution (no binaries) into dist/"
	@echo "  make install             - Build binary and install to ~/.local/bin/arm (adds to PATH)"

GOLANGCI_LINT_MODULE  := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GREMLINS_MODULE       := github.com/go-gremlins/gremlins/cmd/gremlins
GOLANGCI_LINT_VERSION := v2.14.0
GREMLINS_VERSION      := v0.6.0
GOVULNCHECK_VERSION   := v1.1.4
RIPGREP_VERSION       := 15.2.0
SYFT_VERSION          := v1.55.0
DEST ?=

CHECK_JOBS := check-static check-core mutate

CHECK_JOB_check-static := test-check-jobs test-install-verified-release lint crosscompile test-census-drift-check test-coverage-check test-git-test-hermetic-check test-git-test-exec-check test-summarize-test-json test-mock-ban test-source-manifest-decode test-surface-budget-check
CHECK_TOOLS_check-static := golangci-lint

CHECK_JOB_check-core := coverage-check validate-skills validate-doc-examples census-drift-check git-test-hermetic-check git-test-exec-check surface-budget-check
CHECK_TOOLS_check-core :=

CHECK_JOB_mutate := mutate
CHECK_TOOLS_mutate := gremlins

check:
	@scripts/check-commit-subjects.sh origin/main..HEAD
	@set -e; \
	for j in $(CHECK_JOBS); do \
		echo "==> make check-job JOB=$$j"; \
		$(MAKE) check-job JOB=$$j; \
	done

# Full CHECK_JOBS suite without check-commit-subjects. CI's check-git-2-34 leg
# uses this under PATH git 2.34.0: pull_request checkouts are merge SHAs whose
# subjects are unscoped, so bare `make check` dies before S12. Partitioned
# check-job targets do not need origin/main (class rule in quality-gates.md).
check-git-floor:
	@set -e; \
	for j in $(CHECK_JOBS); do \
		echo "==> make check-job JOB=$$j"; \
		$(MAKE) check-job JOB=$$j; \
	done

check-job:
	@set -e; \
	ok=0; \
	for j in $(CHECK_JOBS); do \
		if [ "$(JOB)" = "$$j" ]; then ok=1; fi; \
	done; \
	if [ "$(words $(JOB))" != "1" ] || [ "$$ok" != "1" ]; then \
		echo "FAIL: unknown or empty JOB='$(JOB)' (want one of: $(CHECK_JOBS))" >&2; \
		exit 1; \
	fi; \
	gobin="$$($(GO) env GOBIN)"; \
	if [ -z "$$gobin" ]; then gobin="$$($(GO) env GOPATH)/bin"; fi; \
	export PATH="$$gobin:$$PATH"; \
	for t in $(CHECK_TOOLS_$(JOB)); do \
		case $$t in \
			golangci-lint) mod="$(GOLANGCI_LINT_MODULE)"; ver="$(GOLANGCI_LINT_VERSION)" ;; \
			gremlins) mod="$(GREMLINS_MODULE)"; ver="$(GREMLINS_VERSION)" ;; \
			*) echo "FAIL: unknown check tool $$t" >&2; exit 1 ;; \
		esac; \
		if [ "$(CHECK_INSTALL_TOOLS)" = "1" ]; then \
			echo "Installing $$t@$$ver from GitHub release (checksum-verified)..."; \
			$(MAKE) install-$$t-release DEST="$$gobin"; \
		elif ! command -v $$t >/dev/null 2>&1; then \
			echo "$$t not found. Install with:"; \
			echo "  go install $$mod@$$ver"; \
			echo "Then ensure GOPATH/bin is on your PATH."; \
			exit 1; \
		fi; \
	done; \
	for t in $(CHECK_JOB_$(JOB)); do \
		echo "==> make $$t"; \
		$(MAKE) $$t; \
	done

print-check-jobs:
	@$(foreach j,$(CHECK_JOBS),printf 'job %s\ntargets %s\ntools %s\n' '$(j)' '$(CHECK_JOB_$(j))' '$(CHECK_TOOLS_$(j))';)

print-govulncheck-version:
	@echo $(GOVULNCHECK_VERSION)

print-tool-versions:
	@printf 'GOLANGCI_LINT_VERSION=%s\nGREMLINS_VERSION=%s\nGOVULNCHECK_VERSION=%s\nRIPGREP_VERSION=%s\nSYFT_VERSION=%s\n' \
		'$(GOLANGCI_LINT_VERSION)' '$(GREMLINS_VERSION)' '$(GOVULNCHECK_VERSION)' '$(RIPGREP_VERSION)' '$(SYFT_VERSION)'

define resolve-dest
	dest="$(DEST)"; \
	if [ -z "$$dest" ]; then \
		dest="$$($(GO) env GOBIN)"; \
		if [ -z "$$dest" ]; then dest="$$($(GO) env GOPATH)/bin"; fi; \
	fi
endef

install-golangci-lint-release:
	@$(resolve-dest); \
	scripts/install-verified-github-release.sh \
		--repo golangci/golangci-lint \
		--version "$(GOLANGCI_LINT_VERSION)" \
		--tarball 'golangci-lint-{ver}-{os}-{arch}.tar.gz' \
		--checksums 'golangci-lint-{ver}-checksums.txt' \
		--bin golangci-lint \
		--dest "$$dest"

install-gremlins-release:
	@$(resolve-dest); \
	scripts/install-verified-github-release.sh \
		--repo go-gremlins/gremlins \
		--version "$(GREMLINS_VERSION)" \
		--tarball 'gremlins_{ver}_{os}_{arch}.tar.gz' \
		--checksums checksums.txt \
		--bin gremlins \
		--dest "$$dest"

install-rg-release:
	@$(resolve-dest); \
	os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	machine=$$(uname -m); \
	case "$$os-$$machine" in \
		linux-x86_64) trip=x86_64-unknown-linux-musl ;; \
		linux-aarch64) trip=aarch64-unknown-linux-musl ;; \
		darwin-arm64) trip=aarch64-apple-darwin ;; \
		darwin-x86_64) trip=x86_64-apple-darwin ;; \
		*) echo "FAIL: no ripgrep GitHub asset mapping for $$os-$$machine" >&2; exit 1 ;; \
	esac; \
	tarball="ripgrep-$(RIPGREP_VERSION)-$$trip.tar.gz"; \
	scripts/install-verified-github-release.sh \
		--repo BurntSushi/ripgrep \
		--version "$(RIPGREP_VERSION)" \
		--tarball "$$tarball" \
		--checksums "$$tarball.sha256" \
		--bin rg \
		--dest "$$dest"

install-syft-release:
	@$(resolve-dest); \
	scripts/install-verified-github-release.sh \
		--repo anchore/syft \
		--version "$(SYFT_VERSION)" \
		--tarball 'syft_{ver}_{os}_{arch}.tar.gz' \
		--checksums 'syft_{ver}_checksums.txt' \
		--bin syft \
		--dest "$$dest"

test-install-verified-release:
	@scripts/test_install_verified_github_release.sh .

install-check-tools:
	@set -e; \
	gobin="$$($(GO) env GOBIN)"; \
	if [ -z "$$gobin" ]; then gobin="$$($(GO) env GOPATH)/bin"; fi; \
	export PATH="$$gobin:$$PATH"; \
	echo "Installing golangci-lint@$(GOLANGCI_LINT_VERSION)..."; \
	$(GO) install "$(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)"; \
	echo "Installing gremlins@$(GREMLINS_VERSION)..."; \
	$(GO) install "$(GREMLINS_MODULE)@$(GREMLINS_VERSION)"

test-check-jobs:
	@scripts/test_check_jobs.sh .

validate-graph: build
	@./bin/arm validate --ci

trace-report:
	@$(PYTHON) scripts/trace_report.py .

context-report: build
	@./bin/arm context-report --format human

TEST_TIMEOUT ?= 20m

TEST_JSON_OUT ?=

test: build
	@tmp=$$(mktemp "$${TMPDIR:-/tmp}/armature-test.XXXXXX"); \
	ARM_BIN=$(ARM_BIN) $(GO) test -json -count=1 -timeout $(TEST_TIMEOUT) $(UNIT_PACKAGES) > "$$tmp"; status=$$?; \
	if [ -n "$(TEST_JSON_OUT)" ]; then \
		mkdir -p "$$(dirname "$(TEST_JSON_OUT)")"; \
		cp "$$tmp" "$(TEST_JSON_OUT)"; \
	fi; \
	TEST_TIMEOUT=$(TEST_TIMEOUT) $(PYTHON) scripts/summarize_test_json.py "$$tmp"; summary=$$?; \
	rm -f "$$tmp"; \
	if [ $$status -ne 0 ]; then exit $$status; fi; \
	if [ $$summary -ne 0 ]; then exit $$summary; fi; \
	scripts/check-test-porcelain.sh .

test-ci: build
ifeq ($(GOOS_NATIVE),windows)
	@status=0; \
	for pkg in $(UNIT_PACKAGES); do \
		echo "go test -c $$pkg"; \
		$(GO) test -c -o NUL "$$pkg" || status=1; \
	done; \
	exit $$status
else
	@$(MAKE) test
endif

test-skill-transcript: build
	ARM_BIN=$(ARM_BIN) $(GO) test -v -count=1 ./internal/skilltranscript/...

test-e2eharness: build
	ARM_BIN=$(ARM_BIN) $(GO) test -v -count=1 ./internal/e2e/harness/...

coverage: build
	@tmp=$$(mktemp "$${TMPDIR:-/tmp}/armature-test.XXXXXX"); \
	ARM_BIN=$(ARM_BIN) $(GO) test -json -count=1 -coverprofile=coverage.out $(UNIT_PACKAGES) > "$$tmp"; status=$$?; \
	TEST_TIMEOUT=$(TEST_TIMEOUT) $(PYTHON) scripts/summarize_test_json.py "$$tmp"; summary=$$?; \
	rm -f "$$tmp"; \
	if [ $$status -ne 0 ]; then exit $$status; fi; \
	if [ $$summary -ne 0 ]; then exit $$summary; fi; \
	scripts/check-test-porcelain.sh .
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-check: coverage
	@scripts/coverage-check.sh . coverage.out

lint: adr-principles
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)"; \
		echo "Then ensure ~/go/bin is on your PATH."; \
		exit 1; \
	}
	XDG_CACHE_HOME=/tmp/golangci-lint-cache golangci-lint run ./...
	@scripts/check-mock-ban.sh .
	@$(GO) run ./scripts/repoguard -check source-manifest .

adr-principles:
	@$(PYTHON) scripts/check_adr_principles.py docs/adr

mutate:
	@command -v gremlins >/dev/null 2>&1 || { \
		echo "gremlins not found. Install with:"; \
		echo "  go install $(GREMLINS_MODULE)@$(GREMLINS_VERSION)"; \
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
	@ARM_BIN=$(ARM_BIN) $(PYTHON) scripts/embed_examples.py check

validate-skills: skill-lint embed-examples
	@if grep -rn "make install" internal/skillsembed/skills/*/SKILL.md .agents/skills/*/SKILL.md 2>/dev/null; then \
		echo "FAIL: 'make install' found in skill bodies — remove it or replace with: 'If arm is not found, stop and resolve this before proceeding'"; \
		exit 1; \
	fi
	@echo "Skills and canonical documentation validated: no 'make install' references and no example drift"

validate-doc-examples:
	@go run ./cmd/armature validate doc-examples --repo .

skill-lint: build
	@ARM_BIN=$(ARM_BIN) $(PYTHON) scripts/skill_lint.py .

census-drift-check:
	@scripts/census-drift-check.sh .

test-census-drift-check:
	@scripts/test_census_drift_check.sh .

git-test-hermetic-check:
	@$(GO) run ./scripts/repoguard -check git-hermetic .

test-git-test-hermetic-check:
	@scripts/test_check_git_test_hermetic.sh .

git-test-exec-check:
	@$(GO) run ./scripts/repoguard -check test-executables .

test-git-test-exec-check:
	@scripts/test_check_test_executables.sh .

test-coverage-check:
	@scripts/test_coverage_check.sh .

mock-ban-check:
	@scripts/check-mock-ban.sh .

test-mock-ban:
	@scripts/test_mock_ban.sh .

source-manifest-decode-check:
	@$(GO) run ./scripts/repoguard -check source-manifest .

test-source-manifest-decode:
	@scripts/test_check_source_manifest_decode.sh .

surface-budget-check:
	@$(GO) run ./scripts/surfacebudget -check .

surface-budget-ratchet:
	@$(GO) run ./scripts/surfacebudget -write .

test-surface-budget-check:
	@scripts/test_surface_budget_check.sh .

test-summarize-test-json:
	@$(PYTHON) -m unittest scripts/test_summarize_test_json.py

commit-subjects-check:
	@scripts/check-commit-subjects.sh origin/main..HEAD

check-fast:
	@scripts/check-commit-subjects.sh origin/main..HEAD
	@scripts/check-fast.sh .

test-check-fast:
	@scripts/test-check-fast.sh .

clean:
	rm -rf bin/ dist/ *.out coverage.html mutesting-report/ .claude/skills/ .gemini/skills/ test-results/
	$(GO) clean -testcache

build:
	mkdir -p bin
	GOFLAGS=-buildvcs=false CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/arm$(ARM_EXE) ./cmd/armature

crosscompile:
	@for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "Cross-compiling $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch GOFLAGS=-buildvcs=false CGO_ENABLED=0 $(GO) build -o /dev/null ./... || exit 1; \
	done
	@echo "Cross-compile check passed for all shipped platforms"

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(ARM_BIN) $(INSTALL_DIR)/arm$(ARM_EXE)
	chmod +x $(INSTALL_DIR)/arm$(ARM_EXE)
	@echo "Installed arm to $(INSTALL_DIR)/arm$(ARM_EXE)"
	@echo "Ensure $(INSTALL_DIR) is on your PATH"

deploy-skills:
	@for root in internal/skillsembed/skills .agents/skills; do \
		[ -d "$$root" ] || continue; \
		for name in "$$root"/*/; do \
			[ -d "$$name" ] || continue; \
			name=$$(basename "$$name"); \
			[ -f "$$root/$$name/SKILL.md" ] || continue; \
			for harness in claude gemini codex; do \
				mkdir -p ".$$harness/skills/$$name"; \
				cp -r "$$root/$$name/." ".$$harness/skills/$$name/"; \
			done; \
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
