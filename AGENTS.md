# AGENTS.md

## Tech Stack
- Language: Go (`go 1.25`)
- Module: `github.com/spachava753/cuh`
- Goal: reusable helper packages for computer-use automation (for scripts and CPE agents)

## Essential Commands
- Run all tests: `go test ./...`
- Format all packages: `gofmt -w .`
- Verify module metadata: `go list ./...`

## Documentation
- Project overview and intent: `README.md`
- Root package index docs for discovery via `go doc`: `doc.go`
- Package surface area currently lives directly in source files:
  - `browser/browser.go`
  - `gmail/gmail.go`
  - `macos/messages/messages.go`

## Component Index
- Browser helpers: `browser/`
- Gmail helpers: `gmail/`
- macOS Messages helpers: `macos/messages/`

## Working Notes for Agents
- Keep APIs small and composable; this repo is intended for import into automation scripts.
- Prefer explicit, user-consented operations for machine actions.
- Implement cross-platform behavior only when requested; keep macOS-specific logic in `macos/`.
- Add tests alongside package behavior as implementations replace current stubs.
- Use `github.com/nalgeon/be` for test assertions (`be.Err`, `be.True`, `be.Equal`).
- Prefer `any` over `interface{}` in new and modified code.
