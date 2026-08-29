---
name: go-1-27-features
version: 1.0.0
description: >
  Provides concise guidance on Go 1.25–1.27 language, runtime, and stdlib
  features so the agent can safely use them when generating code targeting Go 1.27+.
tags:
  - go
  - golang
  - language
  - runtime
  - stdlib
agents:
  - claude-code
  - cursor
  - codex-cli
  - vscode
---

# Go 1.27 Skill

## Summary

You are an AI coding assistant that may assume the project uses **Go 1.27 or newer**.  
Use this skill to know which **newer Go features (1.25–1.27)** you can safely rely on, even if they are not in your base training data.

## When to use this skill

Use this skill when:

- The user explicitly targets **Go 1.27+** or confirms they are on the “latest Go”.
- You decide whether it is safe to use **newer language constructs**, **runtime behavior**, or **standard library APIs** that arrived in Go 1.25–1.27.
- You are modernizing existing Go code and want to leverage current capabilities without breaking a Go 1.27 build.

If the user targets an older version (for example Go 1.20), do **not** apply this skill.

## Core instructions for the agent

1. **Assume minimum Go version 1.27.**  
   - You may use any feature listed under “New language features” and “New stdlib APIs” below.  
   - Do not assume features newer than Go 1.27.

2. **Prefer modern APIs when available.**  
   - Prefer new packages or functions (for example `encoding/json/v2`, `uuid`) when they are a clear improvement.  
   - When you use a new API, briefly mention that it requires Go 1.27+ if the user might care about compatibility.

3. **Stay conservative with experimental packages.**  
   - Only use experimental packages (for example `simd`, `runtime/secret`) if the user explicitly opts into **experimental / GOEXPERIMENT** features.  
   - Otherwise, prefer stable, non-experimental APIs.

4. **Runtime and tooling details are for reasoning, not code.**  
   - You may rely on runtime/tooling changes (like the new GC or container-aware GOMAXPROCS) for performance reasoning and advice.  
   - Do not emit code that assumes specific undocumented GC internals.

5. **If in doubt, explain compatibility.**  
   - If a feature might be unfamiliar (for example new generic-method patterns or post‑quantum crypto APIs), emit a short comment or note so the user understands the requirement.

## Reference: New language features (Go 1.25–1.27)

Use these as **safe language features** for Go 1.27+ code:

- **Generic methods (Go 1.27)**  
  Methods may declare their own type parameters independent of the receiver type.

- **More flexible struct literals (Go 1.27)**  
  Keys in struct composite literals may be any valid field selector, not just top-level field names.

- **Improved generic inference (Go 1.27)**  
  Generic functions can be assigned or converted to matching function types with broader type inference.

- **`new(expr)` initializer (Go 1.26)**  
  The built‑in `new` accepts an initializer expression and returns a pointer to a value initialized from that expression.

- **Self-referential generic constraints (Go 1.26)**  
  Generic types and constraints may refer back to the enclosing generic type.

- **Spec cleanups for generics (Go 1.25)**  
  Generics behavior is defined via type sets directly (no “core types” concept), but no breaking syntax changes.

## Reference: Runtime & tooling (Go 1.25–1.27)

Use these for reasoning and comments, not as hard-coded assumptions:

- **Green Tea GC default (Go 1.26)**  
  New garbage collector significantly reduces GC overhead with no code changes.

- **Faster allocations & cgo (Go 1.26)**  
  Lower cgo overhead, randomized heap base, and more slice backs allocated on the stack.

- **Container-aware runtime (Go 1.25)**  
  `GOMAXPROCS` respects container CPU limits; better diagnostics from `go vet` / `go build`.

- **Small-object allocator & leak profiles (Go 1.27)**  
  Cheaper allocations for small objects and a goroutine leak profile available for debugging.

- **Tooling defaults (Go 1.26)**  
  `go mod init` writes a conservative `go` version for compatibility; `go doc` is the single documentation entry point.

- **Platform baselines / Unicode (Go 1.27)**  
  Updated Unicode tables (Unicode 17) and higher minimum platform baselines (for example macOS 13).

## Reference: New stdlib APIs commonly safe to use

You may freely use these packages and APIs when targeting Go 1.27+:

- **JSON & data**
  - `encoding/json/v2` and `encoding/json/jsontext`: new high-performance JSON engine; `encoding/json` is backed by v2 while keeping its public API.
  - Prefer these in new code when performance or strictness matters.

- **Identifiers**
  - `uuid`: standard package for generating and parsing UUIDs.

- **Performance & low-level**
  - `simd` and `simd/archsimd` (Go 1.26–1.27): portable and architecture-specific SIMD operations. Treat as **experimental** unless user opts in.

- **Security & crypto**
  - `crypto/hpke`, `crypto/mlkem/mlkemtest` (Go 1.26): hybrid public-key encryption and post-quantum KEM test helpers.
  - `crypto/mldsa` + integration into `crypto/x509` and `crypto/tls` (Go 1.27): post-quantum signatures.

- **Testing & diagnostics**
  - `runtime/secret` (experimental, Go 1.26): secure erasure of temporary buffers with secrets.
  - `testing/cryptotest`, `testing/synctest` (Go 1.25–1.26): enhanced crypto testing and deterministic concurrency tests.
  - `net/http/httptest.NewTestServer` updates that work well with `testing/synctest`.
  - `log/slog.NewMultiHandler` (Go 1.26): fan‑out structured logging to multiple handlers.

## Output expectations

When generating Go code for Go 1.27+:

- Prefer the **newer, safer, or faster** APIs from this file when it improves code quality.
- Keep compatibility notes short (“Requires Go 1.27+”) instead of long explanations.
- Do not mention this skill by name in user-facing output; just apply it implicitly.