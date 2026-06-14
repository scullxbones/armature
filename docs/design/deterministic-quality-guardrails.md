# Deterministic Quality Guardrails for Spec-Driven Development

**Audience:** Architecture Guild **Status:** Proposed for review **Scope:** Java, TypeScript, Python services and libraries **Decision requested:** Endorse the recommendations below as default standards for new services; adopt the ratchet strategy (§9) for existing codebases.

---

## 1\. Purpose

As code authorship shifts toward AI agents and high-velocity human contribution, review capacity becomes the bottleneck. The strategy proposed here is to convert as much correctness verification as possible into **deterministic, machine-checkable gates** in the build pipeline — signals that are red or green with no judgment call, equally trustworthy whether the author is a senior engineer or an agent.

Two principles drive every recommendation:

1. **Verify behavior against the specification, not the implementation against itself.** Tests that mirror implementation structure pass when the code does what the code does — a tautology.  
2. **Verify the verifier.** Coverage, test counts, and green builds are weak proxies. Where possible, deploy gates that measure whether the test suite would actually detect defects.

Each recommendation below states the control, the supporting evidence, and the concrete implementation per language.

---

## 2\. R1 — Prefer fakes over mocks; enforce by lint

**Recommendation:** Ban mocking frameworks in unit tests for code we own. Each port (in hexagonal terms) gets exactly one shared, contract-verified fake (§3). Mocking-library imports are blocked by lint. Narrow, explicitly documented exemptions are permitted for third-party SDKs that cannot be wrapped behind a port.

**Rationale and evidence:**

- Interaction-based tests ("verify method X was called with Y") couple tests to call sequences. Refactors that preserve behavior break them, and tests that merely verify the mock can pass while proving nothing about the system. Google's engineering guidance (*Software Engineering at Google*, Winters et al., 2020, ch. 13\) explicitly ranks test doubles: real implementation, then fake, then mock — and recommends state testing over interaction testing for this reason. Google's Testing on the Toilet series calls interaction-coupled tests "change-detector tests" and recommends against them.  
- Empirical study of mock usage in mature OSS Java projects (Spadini et al., *To Mock or Not to Mock?*, MSR 2017; extended in EMSE 2019\) found mocks concentrated on infrastructure dependencies and identified maintenance coupling between mock setups and production code evolution.  
- The failure mode is amplified by AI-generated tests, which readily produce interaction-verifying tests that assert the mock was called — high coverage, zero defect-detection power.

**Hexagonal fit:** With ports/adapters, the fake-per-port model is structurally natural: the domain only sees port interfaces, so a small set of reusable fakes (in-memory repository, recording message bus, fixed clock) covers the entire unit-test surface. Outbound side effects are asserted via recording fakes rather than mock verification.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Ban mechanism | Checkstyle `IllegalImport` / ArchUnit rule against `org.mockito..`, `org.easymock..` | ESLint `no-restricted-imports` for `jest.mock`, `sinon`, `ts-mockito`; ESLint `no-restricted-properties` for `jest.spyOn` | Ruff/Flake8 banned-imports (e.g. `flake8-tidy-imports`) for `unittest.mock`, `pytest-mock`; explicitly ban `mock.patch` |
| Fake pattern | One fake class per port interface, shipped in a `testFixtures` source set (Gradle) for reuse | Fake class per port interface exported from a `/testing` entry point | Fake class per `Protocol`/ABC port, shipped in a `testing` subpackage |

Note: banning the library does not prevent hand-rolled mocks. The structural control that makes this sound is R2.

---

## 3\. R2 — Contract-verify every fake

**Recommendation:** Every fake must pass the same abstract test suite as the real adapter it stands in for. The suite is written once against the port contract and executed against both implementations; the real-adapter run uses Testcontainers or equivalent.

**Rationale:** An unverified fake is a centralized assumption; if it drifts from real adapter semantics (ordering, error types, null handling, idempotency), every unit test built on it silently tests the wrong contract. Contract verification is what makes R1 strictly stronger than mocking — a fake is *checked* against reality; a mock never is. This is the in-process analogue of consumer-driven contract testing (Pact), which we separately recommend at service boundaries.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Shared suite | Abstract JUnit 5 class with `@TestFactory`/template methods; concrete subclasses bind fake vs. Testcontainers-backed adapter | Shared `describe` factory function parameterized by an implementation provider (Jest/Vitest) | Pytest fixture parameterization: one test module, fixture yields fake or real adapter |
| Real-adapter backing | Testcontainers | Testcontainers (Node) | testcontainers-python |
| Enforcement | ArchUnit: every class implementing a port in test scope must extend the contract suite | Lint/convention \+ CI check that each `*Fake` has a matching contract spec file | Convention \+ a small custom check: every `Fake*` class is referenced by a contract test module |

---

## 4\. R3 — Purity rules in the domain core

**Recommendation:** Domain code may not access the clock, randomness, environment, network, or filesystem directly. These are ports (`Clock`, `IdGenerator`, config injected at the boundary). Enforced by import/architecture lint, not convention.

**Rationale:** Non-determinism in the core is the root cause of the dominant categories of flaky tests — Luo et al. (FSE 2014\) found async waits, concurrency, and test-order dependency to be the leading causes of flakiness in their large OSS study. A pure core makes fakes trivial (R1), tests reproducible, and property-based testing (R7) practical. It is also the load-bearing wall of hexagonal architecture: if the domain can reach `DateTime.now()`, the hexagon leaks.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Time | Inject `java.time.Clock`; Error Prone / ArchUnit rule against `Instant.now()`, `LocalDateTime.now()` (no-arg) in domain packages | Inject clock port; ESLint `no-restricted-syntax` for `Date.now`, `new Date()` in domain dirs | Inject clock port; Ruff/custom rule against `datetime.now`, `time.time` in domain packages |
| Randomness / IDs | Ban `new Random()`, `UUID.randomUUID()` in domain; inject generator | Ban `Math.random`, `crypto.randomUUID` in domain | Ban `random`, `uuid.uuid4` in domain |
| Env/config | Ban `System.getenv` outside composition root (ArchUnit) | Ban `process.env` outside config module (ESLint boundaries) | Ban `os.environ` outside settings module |

---

## 5\. R4 — Architecture conformance as a build gate

**Recommendation:** Encode the dependency rules of the architecture (domain imports nothing from adapters; adapters do not import each other; modules respect declared boundaries) as executable rules that fail the build.

**Rationale:** Architecture erodes one expedient import at a time, and human review reliably misses it. Executable conformance rules make the intended design self-enforcing and give agents an unambiguous signal when they violate structure. Google's experience deploying static analysis at scale (Sadowski et al., *Lessons from Building Static Analysis Tools at Google*, CACM 2018\) found that checks are effective precisely when integrated into the developer workflow as blocking, low-false-positive feedback — which architecture rules are.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Tooling | **ArchUnit** (layer/slice rules as JUnit tests); Spring Modulith `verify()` where applicable; Gradle/Maven module boundaries as first line | **dependency-cruiser** rule file or **eslint-plugin-boundaries**; `tsconfig` project references for hard module isolation | **import-linter** contracts (layers, independence, forbidden) |

---

## 6\. R5 — Maximize the type system; gate at strict

**Recommendation:** Strictest practical type configuration is the default for new code: TypeScript `strict` plus `noUncheckedIndexedAccess`; Python `mypy --strict` or Pyright strict; Java augmented with Error Prone and NullAway. Domain concepts get dedicated types (no stringly-typed IDs), turning spec constraints into compile errors.

**Rationale and evidence:** Gao, Bird & Barr (*To Type or Not to Type*, ICSE 2017\) found that adding TypeScript or Flow annotations would have detected \~15% of real, public bug-fix commits in JavaScript projects — a large defect class eliminated by a fully deterministic gate. Uber's NullAway (Banerjee et al., FSE 2019\) demonstrated practical, low-overhead null-safety enforcement for Java at industrial scale. Types are the cheapest guardrail per defect caught: zero flakiness, zero runtime cost, instant feedback.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Gate | Error Prone (error severity) \+ NullAway on compile; `-Werror` | `tsc --noEmit` with `strict: true`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes` | `mypy --strict` or Pyright strict in CI |
| Domain typing | Records, sealed interfaces for state machines; value types for IDs | Branded types for IDs; discriminated unions for state | `NewType`/dataclasses for IDs; `Literal`/tagged unions; `Protocol` ports |

---

## 7\. R6 — Mutation testing as the test-quality gate

**Recommendation:** Adopt mutation testing on changed code as a merge gate (incremental mode), with a ratcheted mutation-score threshold. Treat it as the primary measure of suite quality; treat line coverage as a diagnostic only, never a target.

**Rationale and evidence:**

- Inozemtseva & Holmes (*Coverage Is Not Strongly Correlated with Test Suite Effectiveness*, ICSE 2014\) found that, controlling for suite size, coverage is a weak predictor of fault-detection ability. Coverage measures execution, not assertion strength.  
- Mutation testing measures exactly the property we care about — does the suite fail when the code is defective — and has been deployed at industrial scale: Google integrated mutant feedback into code review across thousands of projects (Petrović & Ivanković, *State of Mutation Testing at Google*, ICSE-SEIP 2018), and Meta reported on practical industrial adoption (Beller et al., ICSE-SEIP 2021). Both report that incremental, diff-scoped application is what makes it tractable.  
- This is the end-to-end check on R1–R3: if fakes plus state-based tests genuinely verify behavior, mutants die. If a suite is interaction-theater, mutants survive and the gate fails — regardless of coverage.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Tool | **PIT (pitest)** with history/incremental analysis | **Stryker** with incremental mode | **mutmut** (or cosmic-ray) scoped to diff |
| Gate | Mutation score on changed lines ≥ baseline (ratchet) | Same | Same |

---

## 8\. R7 — Hermetic, escape-hatch-free tests

**Recommendation:** Unit tests run with no network, no real filesystem outside a temp dir, no sleeps, no wall clock, and no retries. Skipped tests, focused tests, and assertion-free tests fail CI.

**Rationale:** Flaky tests destroy the value of every other gate — a red that might be noise gets ignored, and agents retrain on "rerun until green." Luo et al. (FSE 2014\) attribute the bulk of flakiness to async waits, concurrency, and order dependence; hermeticity plus fake clocks removes the first and third classes structurally. Retry-on-failure is explicitly banned because it converts a deterministic gate back into a probabilistic one.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Network block | JUnit 5 extension failing on socket use in unit scope; route all real I/O to Testcontainers-tagged integration tests | Vitest/Jest setup that stubs `fetch`/`net` to throw in unit projects | **pytest-socket** (`--disable-socket`) |
| Time | Fixed `Clock` (R3) | Fake timers (`vi.useFakeTimers`) \+ clock port | Clock port; `time-machine` only at boundaries |
| No sleeps | Checkstyle/ArchUnit ban on `Thread.sleep` in tests | ESLint ban on `setTimeout` in tests | Ruff ban on `time.sleep` in tests |
| No escape hatches | Build fails on `@Disabled` without linked issue; no rerun plugins | ESLint `no-focused-tests`, `no-disabled-tests`, `expect-expect` (jest/vitest plugins) | Fail on unmarked `skip`/`xfail`; pytest plugin asserting ≥1 assertion per test |
| Test simplicity | Cyclomatic complexity limit ≈1 in test sources | Same via ESLint `complexity` scoped to test files | Same via Ruff `C901` scoped to tests |
| Public surface only | Tests import only exported API (ArchUnit) | Tests import only package entry points (dependency-cruiser) | Tests import only public modules (import-linter) |

---

## 9\. R8 — Specification traceability gate

**Recommendation:** Assign stable IDs to every requirement in the spec/PRD. Tag tests with the IDs they verify. A pipeline step fails the build if (a) any requirement ID lacks at least one passing test, or (b) any test references an unknown ID. Acceptance-level requirements additionally map to executable BDD scenarios where the team uses them.

**Rationale:** This is the direct, deterministic answer to "is the spec fully implemented." Requirements traceability is mandatory and proven in regulated domains (DO-178C avionics, IEC 62304 medical software); the mechanism is simple bookkeeping, but mainstream tooling outside those domains is absent — so this is a small piece of custom pipeline code with outsized leverage, especially for agent-driven work where the spec is the prompt. Honest caveat: the gate verifies *presence and passing* of a mapped test, not its semantic adequacy; R6 (mutation testing) is the complementary control on adequacy, and human review of the spec→test mapping remains the residual manual step.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Tagging | JUnit 5 `@Tag("REQ-123")` | Test title convention `[REQ-123]` or tagged metadata | `@pytest.mark.req("REQ-123")` |
| Reconciliation | Small CI script: parse spec IDs (front-matter in spec repo) ↔ test report tags; fail on orphans either direction | Same | Same |
| Acceptance layer | Cucumber-JVM scenarios keyed by ID | Cucumber.js / Playwright tagged specs | pytest-bdd |

---

## 10\. R9 — Boundary contracts and breaking-change detection

**Recommendation:** All service boundaries are spec-first (OpenAPI/Protobuf/AsyncAPI). The pipeline (a) verifies the implementation conforms to the published schema, and (b) deterministically diffs the schema against the previously released version, failing on breaking changes. Consumer-driven contracts (Pact) where consumer teams are known.

**Rationale:** The API schema is the machine-readable portion of the spec; conformance and compatibility checks against it are fully deterministic and catch the defect class with the widest blast radius (cross-team breakage). This extends R2's fake-contract discipline across process boundaries.

**Implementation:**

|  | Java | TypeScript | Python |
| :---- | :---- | :---- | :---- |
| Conformance | springdoc \+ Schemathesis against running service; Pact provider verification | Schemathesis / Dredd; Pact JS | Schemathesis (native fit with FastAPI); Pact Python |
| Breaking-change diff | `oasdiff` (REST), `buf breaking` (Protobuf) | Same | Same |

---

## 11\. Adoption strategy: ratchets, not cliffs

Imposing absolute thresholds on existing codebases produces gaming or revolt. Adopt every quantitative gate as a **ratchet**: the metric on changed code must meet or exceed the baseline recorded on the main branch, and the baseline only moves up.

- **New services:** all gates on from day one; mock ban absolute.  
- **Existing services:** R4 (architecture) and R5 (types) first — they are diff-scopable and fail loudly with near-zero false positives; then R7 (hermeticity), then R6 (mutation, incremental mode only), then R1/R2 applied to new and modified tests, then R8 for new feature work.  
- **Exemptions:** any suppression (lint ignore, `@Disabled`, type ignore) requires a linked issue; a weekly CI job counts suppressions and fails on increase.

## 12\. Summary of expected effect

| Gate | Defect class removed | Determinism |
| :---- | :---- | :---- |
| R5 types | Null/shape/state-machine errors (\~15% of JS field bugs per ICSE 2017\) | Total |
| R4 architecture | Structural erosion, layering violations | Total |
| R3 purity \+ R7 hermeticity | Flaky tests, irreproducible failures | Total |
| R1 fakes \+ R2 contracts | Tautological tests, fake/real drift | Total |
| R6 mutation | Weak assertions, coverage theater | Total (seeded) |
| R8 traceability | Unimplemented requirements | Total for presence; adequacy via R6 |
| R9 boundary contracts | Cross-service breakage | Total |

## 13\. References

- Winters, Manshreck, Wright. *Software Engineering at Google*. O'Reilly, 2020 — test-double preference ordering; state vs. interaction testing.  
- Spadini et al. "To Mock or Not to Mock? An Empirical Study on Mocking Practices." MSR 2017; extended EMSE 2019\.  
- Inozemtseva, Holmes. "Coverage Is Not Strongly Correlated with Test Suite Effectiveness." ICSE 2014\.  
- Petrović, Ivanković. "State of Mutation Testing at Google." ICSE-SEIP 2018\.  
- Beller et al. "What It Would Take to Use Mutation Testing in Industry — A Study at Facebook." ICSE-SEIP 2021\.  
- Gao, Bird, Barr. "To Type or Not to Type: Quantifying Detectable Bugs in JavaScript." ICSE 2017\.  
- Banerjee, Clapp, Sridharan. "NullAway: Practical Type-Based Null Safety for Java." ESEC/FSE 2019\.  
- Luo et al. "An Empirical Analysis of Flaky Tests." FSE 2014\.  
- Sadowski et al. "Lessons from Building Static Analysis Tools at Google." CACM, 2018\.  
- RTCA DO-178C; IEC 62304 — requirements traceability practice in regulated software.

