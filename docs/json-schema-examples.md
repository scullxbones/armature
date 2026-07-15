# Armature JSON Schema Examples

This document provides examples of Armature's JSON artifact types for reference and validation testing.

## Plan Schema

A plan file describes issues to be created or modified by decomposition.

```json artifact_type=plan
{
  "version": 1,
  "title": "Feature implementation plan",
  "issues": [
    {
      "id": "FEATURE-S1-T1",
      "title": "Implement core logic",
      "type": "task",
      "scope": "src/main.go",
      "priority": "high",
      "dod": "Core logic implemented, tested, and documented",
      "parent": "FEATURE-S1",
      "blocked_by": [],
      "notes": ["This is the foundation for the rest of the feature"],
      "acceptance": ["All unit tests pass", "Code meets style guidelines", "Documentation is complete"]
    }
  ]
}
```

## Review Bundle Schema

A review bundle is the input prepared by `arm review prepare` and consumed by the reviewer.

```json artifact_type=review_bundle
{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "issue": {
    "id": "FEATURE-S1-T1",
    "type": "task",
    "title": "Implement core logic",
    "outcome": "Implemented authentication middleware with comprehensive test coverage"
  },
  "contract": {
    "definition_of_done": "Core logic is implemented, fully tested, and documented",
    "scope": ["src/auth.go", "src/auth_test.go"],
    "acceptance": ["All unit tests pass with >90% coverage", "Authentication flow works end-to-end", "Error cases are handled gracefully"]
  },
  "delivery": {
    "base_sha": "1234567890abcdef1234567890abcdef12345678",
    "head_sha": "abcdef1234567890abcdef1234567890abcdef12",
    "changed_files": ["src/auth.go", "src/auth_test.go", "docs/auth.md"],
    "diff": "--- a/src/auth.go\n+++ b/src/auth.go\n@@ -0,0 +1,42 @@\n..."
  },
  "fingerprints": {
    "contract": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "delivery": "5d41402abc4b2a76b9719d911017c592250c71ed373cade4e832627b4f6c043a"
  }
}
```

## Conformance Assessment Schema

A conformance assessment is the detailed result returned by a reviewer evaluating a delivery against its contract.

```json artifact_type=conformance_assessment
{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "results": [
    {
      "id": "definition_of_done",
      "status": "satisfied",
      "rationale": "The implementation includes full test coverage (92%) and comprehensive documentation",
      "citations": [
        {"path": "src/auth_test.go", "line": 1},
        {"path": "docs/auth.md", "line": 1}
      ]
    },
    {
      "id": "acceptance[0]",
      "status": "satisfied",
      "rationale": "Unit tests pass with 92% coverage, exceeding the 90% requirement",
      "citations": [
        {"path": "src/auth_test.go", "line": 150}
      ]
    },
    {
      "id": "acceptance[1]",
      "status": "satisfied",
      "rationale": "Authentication flow tested end-to-end in integration tests",
      "citations": [
        {"path": "src/auth_test.go", "line": 250}
      ]
    },
    {
      "id": "acceptance[2]",
      "status": "satisfied",
      "rationale": "Error handling is comprehensive with custom error types",
      "citations": [
        {"path": "src/auth.go", "line": 35}
      ]
    }
  ],
  "contract_fingerprint": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "delivery_fingerprint": "5d41402abc4b2a76b9719d911017c592250c71ed373cade4e832627b4f6c043a"
}
```

## Activity Index Schema

An activity index summarizes the execution activity log for reviewer navigation.

```json artifact_type=activity_index
{
  "schema_version": 1,
  "log_path": "/path/to/armature-activity.log",
  "log_digest": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "entry_count": 5,
  "delivery_head_count": 5,
  "earlier_count": 0,
  "entries": [
    {
      "id": "0",
      "command": "go build ./cmd/armature",
      "exit_status": 0,
      "head_anchor": true,
      "category": "build",
      "log_pointer": "0"
    },
    {
      "id": "1",
      "command": "go test ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "1"
    },
    {
      "id": "2",
      "command": "golangci-lint run ./...",
      "exit_status": 0,
      "head_anchor": true,
      "category": "lint",
      "log_pointer": "2"
    },
    {
      "id": "3",
      "command": "make coverage-check",
      "exit_status": 0,
      "head_anchor": true,
      "category": "test",
      "log_pointer": "3"
    },
    {
      "id": "4",
      "command": "git commit -m 'feat: implement auth middleware'",
      "exit_status": 0,
      "head_anchor": true,
      "category": "other",
      "log_pointer": "4"
    }
  ]
}
```

## Example with Unknown Exit Status

Activity entries may have unknown exit status when the harness could not determine completion:

```json artifact_type=activity_index
{
  "schema_version": 1,
  "log_path": "/path/to/armature-activity.log",
  "log_digest": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "entry_count": 1,
  "delivery_head_count": 1,
  "earlier_count": 0,
  "entries": [
    {
      "id": "0",
      "command": "make check",
      "exit_status": "unknown",
      "head_anchor": true,
      "category": "test",
      "log_pointer": "0"
    }
  ]
}
```
