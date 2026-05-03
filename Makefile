.PHONY: test coverage coverage-check lint clean mutate check help skill dist-skills install

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
	@echo "  make skill      - Build binary and deploy all skills/ to .claude/ and .gemini/"
	@echo "  make dist-skills - Package skills for distribution (no binaries) into dist/"
	@echo "  make install    - Build binary and install to ~/.local/bin/arm (adds to PATH)"

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
	CGO_ENABLED=0 go build -ldflags "-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o bin/arm ./cmd/armature

install: build
	mkdir -p ~/.local/bin
	cp bin/arm ~/.local/bin/arm
	chmod +x ~/.local/bin/arm
	@echo "Installed arm to ~/.local/bin/arm"
	@echo "Ensure ~/.local/bin is on your PATH"

deploy-skills:
	@for name in internal/skillsembed/skills/*/; do \
		name=$$(basename "$$name"); \
		[ -f "internal/skillsembed/skills/$$name/SKILL.md" ] || continue; \
		for harness in claude gemini; do \
			mkdir -p ".$$harness/skills/$$name"; \
			cp "internal/skillsembed/skills/$$name/SKILL.md" ".$$harness/skills/$$name/SKILL.md"; \
			if [ -d "internal/skillsembed/skills/$$name/scripts" ]; then \
				mkdir -p ".$$harness/skills/$$name/scripts"; \
				cp "internal/skillsembed/skills/$$name/scripts/"* ".$$harness/skills/$$name/scripts/"; \
				chmod +x ".$$harness/skills/$$name/scripts/"*; \
			fi; \
			if [ -d "internal/skillsembed/skills/$$name/references" ]; then \
				mkdir -p ".$$harness/skills/$$name/references"; \
				cp "internal/skillsembed/skills/$$name/references/"* ".$$harness/skills/$$name/references/"; \
			fi; \
		done; \
	done
	@echo "Deployed skills to .claude/skills/ and .gemini/skills/"

skill: build deploy-skills

dist-skills:
	mkdir -p dist
	@for harness in claude gemini; do \
		python3 -c "\
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
