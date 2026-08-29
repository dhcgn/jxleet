---
name: go-release
description: >
  Expert skill for releasing Go modules and applications: semantic versioning, API compatibility,
  breaking-change detection, deprecation, go.mod hygiene, tagging, and binary distribution with
  GoReleaser. Use whenever a Go release is being cut, planned, or reviewed — any mention of
  version tags, semver, breaking changes, v2 modules, deprecating an API, editing go.mod for
  release, changelogs, or shipping binaries. Also use when reviewing a PR or spec that changes
  any exported identifier in a published Go library.
---

# Go Release Engineering: Versions, Compatibility, and Distribution

A release is a promise. Once a version is tagged and fetched through the module proxy, it is immutable and someone depends on it forever. This skill covers how to make that promise correctly — for libraries (API compatibility) and applications (binary distribution).

## When to Activate

- Cutting or planning a release of a Go module or binary
- Reviewing whether a change is breaking for a published library
- Deprecating or removing exported API
- Editing `go.mod` in a way that affects dependents (go directive, dependencies)
- Setting up binary distribution (GoReleaser, ldflags, multi-platform builds)
- Recovering from a bad release

## The Compatibility Promise

### Versioning Semantics in Go

- **v0.x.y** — no compatibility promise. Break freely, but document breaks in release notes. Staying on v0 indefinitely is a cop-out for a widely-used library: dependents deserve a promise.
- **v1.x.y** — the promise: code written against v1.0.0 compiles against every later v1 release. Minor versions add, patches fix. Nothing is removed or changed incompatibly. Ever.
- **v2+** — a *different module* with a different import path (`module github.com/you/lib/v2`, imported as `.../lib/v2`). This is semantic import versioning and it is not optional — a v2 tag without the `/v2` module path is broken for module-mode consumers.

**Plan to stay on v1 forever.** A major version bump splits your ecosystem: every dependent must migrate manually, and transitive dependencies mix v1 and v2 in the same build. Cobra, pflag, and the standard library have held v1 for a decade by expanding rather than replacing. Reach for v2 only when the API is genuinely unfixable additively — and treat it as launching a new library, because that's what it costs.

### What Is Actually a Breaking Change

LLMs and humans both misjudge this. The compiler-visible breaks:

| Change | Breaking? | Why |
|--------|-----------|-----|
| Removing/renaming any exported identifier | **Yes** | Obvious |
| Changing a function/method signature | **Yes** | Includes adding a variadic parameter — the signature changes |
| **Adding a method to an exported interface** | **Yes** | Every external implementor no longer satisfies it |
| Adding a field to a struct | Usually no | **Breaks callers using unkeyed composite literals** and exact-type assignments; safe if you've documented keyed-literals-only |
| Adding a method to a struct | No | Unless it collides with a method promoted from an embedded type in *their* code — rare, accepted risk |
| Adding a new function, type, or package | No | This is what minor versions are for |
| Changing an untyped constant's value | **Yes** | Compiled into caller binaries |
| Tightening input validation | **Behaviorally yes** | Code that worked now errors — treat as breaking |
| Changing error *messages* | No | But breaking for anyone string-matching; give them `errors.Is`-able sentinels instead |
| Raising the `go` directive in go.mod | **Effectively yes** | Dependents on older toolchains can no longer build you — see go.mod hygiene |

**The interface trap deserves emphasis:** if your library exports an interface that users implement, it is frozen at v1. Design for this — either keep exported interfaces tiny and stable, or add an unexported method so only you can implement it, or accept functions/structs instead of defining the interface at all (interfaces belong to consumers — see the `go` skill).

### Verify Mechanically, Not by Eyeball

```bash
# Before every tag — compares against the latest released version
go run golang.org/x/exp/cmd/gorelease@latest

# Or compare two specific API surfaces
go run golang.org/x/exp/cmd/apidiff@latest old/ new/
```

`gorelease` reports incompatible changes and suggests the correct next version number. Run it in CI on every PR that touches exported identifiers — catching a break at review time is free; catching it after the tag costs a major version or a retraction.

## Deprecation: The Additive Escape Hatch

You can't remove, but you can deprecate. The convention is machine-readable and respected by gopls, staticcheck, and pkg.go.dev:

```go
// Deprecated: Use [ParseContext] instead, which supports cancellation.
// This function will be maintained but not extended.
func Parse(input string) (*Result, error) { ... }
```

- The paragraph must begin exactly with `Deprecated:` to be recognized.
- Deprecated API **keeps working forever** on v1 — deprecation redirects new code; it does not license removal.
- Always name the replacement. A deprecation without a migration path is just an insult.
- Deprecate in a minor release, never silently in a patch.

## go.mod Hygiene for Releases

Your go.mod is part of your API. Through minimal version selection, your minimums become your dependents' minimums.

- **The `go` directive is a floor you impose on every dependent.** Set it to the oldest version you actually need, not the newest you happen to run. Bumping it just to use one new stdlib helper forces an upgrade on your whole downstream tree — for a library, that's a decision, not a default. (The `toolchain` directive is local preference and doesn't constrain dependents.)
- **Never tag a release containing `replace` directives.** They are ignored in dependents' builds, so the module you tested is not the module they get. Same for `go.work` assumptions — workspaces don't ship.
- **Run `go mod tidy` before tagging.** A dirty go.mod ships phantom requirements to everyone downstream.
- **Keep dependency minimums low.** Requiring the latest patch of every dependency forces churn on dependents; require the oldest version that has what you need.

## The Release Checklist

```bash
go mod tidy && git diff --exit-code go.mod go.sum   # no surprises
go build ./...
go vet ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go run golang.org/x/exp/cmd/gorelease@latest        # libraries: API check + version suggestion
```

Then:

```bash
git tag -a v1.4.0 -m "v1.4.0"    # annotated tag, full semver, leading v
git push origin v1.4.0
```

- Tag the module root. In a multi-module repo, prefix with the module directory: `submod/v1.4.0`.
- Write release notes for humans: what's new, what's deprecated, any behavior changes. Link migration guidance for anything deprecated.

### The Proxy Is Forever: Fixing a Bad Release

The module proxy caches every tagged version permanently. **Never delete or re-tag a published version** — the proxy still serves the original, and now your tag and the proxy disagree, which breaks checksum verification for everyone.

The correct fix is `retract`:

```go
// go.mod of a NEW release (e.g. v1.4.2)
retract (
    v1.4.1 // Broke Parse on empty input; use v1.4.2.
)
```

Ship the retraction in the next release. `go get` skips retracted versions and `go list -m -u` warns users already on them. A version can even retract itself (`retract v1.5.0` inside v1.5.0's go.mod, published as v1.5.1... which retracts both). Retraction is advisory withdrawal — the only kind that exists.

## Application Releases: Shipping Binaries

For CLIs and services, the release artifact is a binary, and **GoReleaser is the standard** — don't hand-roll cross-compilation matrices in shell scripts or Makefiles.

### Version Injection

Wire build-time version info into the variables your CLI already exposes (see the `cobra-viper` skill's Version handling):

```yaml
# .goreleaser.yaml
builds:
  - env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags:
      - -s -w
      - -X github.com/you/app/cmd.version={{.Version}}
      - -X github.com/you/app/cmd.commit={{.ShortCommit}}
      - -X github.com/you/app/cmd.date={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
```

- `CGO_ENABLED=0` for static, portable binaries — enable cgo only when a dependency truly requires it.
- `-trimpath` strips your local paths from the binary: smaller, reproducible, no leaked `/home/you/src/...` in panics.
- `-s -w` strips symbol tables — standard for release binaries; omit if you want usable core dumps.

Simpler fallback: `debug.ReadBuildInfo()` returns the module version and VCS revision with zero ldflags when installed via `go install` — a reasonable default before you need GoReleaser at all.

### Distribution Checklist

- **Checksums:** GoReleaser emits `checksums.txt` by default — publish it.
- **Signing/provenance:** cosign keyless signing or an SLSA provenance workflow once your project has enough users that supply-chain questions arrive. Don't cargo-cult this into a weekend project.
- **`go install` must always work:** `go install github.com/you/app@latest` is the zero-infrastructure install path. Keep `main` at the repo root or under `cmd/app` so the path is guessable, and never break it with replace directives.
- **Package managers** (Homebrew tap, nfpm for deb/rpm, Scoop): GoReleaser generates all of these from config — add them when users ask, not before.

## Common Mistakes

- **Adding a method to an exported interface in a minor release** — the classic silent break. Run `gorelease`; it catches this.
- **Re-tagging after a botched release** — the proxy already has the old bytes; you've now broken checksums globally. Retract and roll forward.
- **Tagging v2.0.0 without changing the module path to `/v2`** — the tag exists but module-mode consumers can't use it.
- **Bumping the go directive casually** — you just forced a toolchain upgrade on every dependent.
- **Shipping a release with `replace` in go.mod** — dependents get different dependency versions than you tested.
- **Treating error message text as stable API in docs/tests** — export sentinel errors; message wording is yours to change.
- **Hand-rolled `GOOS`/`GOARCH` build loops in CI** — GoReleaser exists; a bespoke matrix is complexity with no payoff.
- **Skipping `go mod tidy` before the tag** — the one hygiene step that can't be fixed after publishing.