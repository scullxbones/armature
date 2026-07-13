# Keep semantic conformance review advisory and skill-driven

Status: amended by ADR-0008 (harness-recorded execution evidence is now admissible review input, upgrade-only; hooks still neither initiate review nor gate on it)

## Principles touched

I5, N4

Armature will provide a versioned protocol for preparing semantic-review bundles, validating structured LLM-as-judge results, deriving ratings, and recording compact attestations, while a fresh `armature-reviewer` skill performs the judgment. Phase one will not run models or customer checks inside Armature, use harness hooks or activity traces, retain full reports in Git, or block delivery on review results. This preserves Armature's role as a git-native coordination and evidence system, keeps subjective evaluation separate from deterministic policy, and avoids reintroducing provider-coupled managed execution.
