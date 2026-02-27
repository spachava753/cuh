# AGENTS.md

## Tech Stack
- Language: Go (`go 1.25`)
- Module: `github.com/spachava753/cuh`
- Goal: reusable helper packages for computer-use automation (for scripts and CPE agents)

## Working Notes for Agents
- Implement cross-platform behavior only when requested; keep macOS-specific logic in `macos/`.
- Use `github.com/nalgeon/be` for test assertions (`be.Err`, `be.True`, `be.Equal`).
- Prefer `any` over `interface{}` in new and modified code.

## Cognitive Framework for New/Refactored Packages

Use this framework when building or refactoring packages in `cuh` (modeled after the Gmail primitive refactor).

1) Intent Model First
- Define what user intent the package should help agents execute on a machine.
- Write the minimal end-to-end flow in plain language before coding: `find/select -> read/hydrate -> decide -> mutate/send/act`.
- Keep the package domain-focused (avoid cross-domain leakage).

2) Primitive-First API Design
- Expose orthogonal primitives, not recipes.
- Recipes (e.g., "archive newsletters") should be composition examples built on primitives, not core exported API.
- Prefer typed inputs/outputs over stringly-typed DSLs.
- Use stable references/identifiers so outputs from one primitive can feed directly into others.

3) Agent-Centric Discoverability Contract
- Package-level godoc must explain: purpose, primitive set, safety model, and composition pattern.
- Every exported symbol (func/type/const/var) must have precise godoc.
- Include composition examples in godoc for complex, real workflows.
- Assume agents rely on `go doc` only; docs should be sufficient without source inspection.

4) Composability Rules
- Design primitives so they chain naturally with minimal glue code.
- Keep read and write paths explicit and separated.
- Support pagination/continuation for list/find operations.
- Return structured per-item results for batch operations (enables partial-success reasoning).

5) Safety and Control Surfaces
- Favor explicit side effects; avoid hidden mutations.
- Add `DryRun` for mutating/transmission primitives whenever practical.
- Make scope explicit in APIs (entity type, target refs, operation values).
- Prefer idempotent behavior where possible and document non-idempotent behavior clearly.

6) Testing Strategy
- Add fast unit/compile tests for API shape and composition snippets.
- Add live integration tests for real external systems when applicable.
- Gate live tests with environment variables (no build tags required unless necessary).
- Validate lifecycle paths for core primitives (list/find, get/read, mutate, send/act, cleanup/delete).

7) Refactor Discipline
- On major API rewrites, remove obsolete APIs instead of keeping ambiguous overlaps (unless compatibility is requested).
- Ensure docs and tests move with the new surface immediately.
- Keep migration notes/examples in docs when old patterns existed.

## Definition of Done for a Package
- Primitive set is minimal, orthogonal, and sufficient for common recipes.
- API is highly introspectable via `go doc` at package and symbol levels.
- Complex compositions are demonstrated with tested examples.
- Safety surfaces (`DryRun`, explicit scope) are present for side effects.
- Unit tests pass; live tests are available and gated for real-system validation.
