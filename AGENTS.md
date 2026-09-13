# AGENTS.md — jxleet

Guidance for AI agents and contributors working on this repository.

## What this is
**jxleet** is a Windows GUI/CLI front end for libjxl's `cjxl`. **It encodes nothing
itself** — it decides which files to hand over, assembles the arguments, runs the
process, verifies, and reports. Never add encoding logic; only orchestrate the
libjxl tools.

- `README.md` — product specification, with a screenshot per view.
- `ARCHITECTURE.md` — how the system is built. Read before structural work.
- `docs/GLOSSARY.md` — normative project vocabulary (`jl:` keys, generated
  from source comments via `go generate ./...`; chapters in
  `docs/GLOSSARY.topics.yaml`). Agents and contributors SHOULD use a `jl:` key
  in issues / PRs / discussions whenever a defined term exists, and point at
  one with `ref:<key>` from source comments or Markdown prose. A `ref:` with
  no definition fails generation (Broken references section) — fix by defining
  the key or correcting the reference. When a term is
  ambiguous, ask first, then fix the glossary description — never silently
  redefine a term. There is deliberately no CI gate on prose wording.
  New term? Define it once in code at the file that implements it, and use
  `ref:` everywhere else including docs (see the `glossary-create` skill).

## Current state
Core complete: routes engine (transcode/reencode/encode), YAML presets with
verbatim cjxl args, output policies with verified recycle-bin replace, managed
libjxl toolchain, single-instance named-pipe IPC with handover/coalescing, the
Svelte/Wails GUI (all views including History), CLI path invocation, Explorer
context menu, append-only history, interactive collision prompt. `task check`
is green.

**Open (pick from here):**
- Log view with tool messages; persistent run logs
- Full multi-rule preset → global-controls sync (session-only overrides today)

## Locked decisions (do not silently change)
- **Windows 10/11 x64 only** for v1. Core features are Windows-specific (recycle
  bin, Explorer context menu, named-pipe handover, `%APPDATA%`/`%LOCALAPPDATA%`).
- **Managed toolchain** = libjxl GitHub release asset `jxl-x64-windows-static.zip`,
  integrity-verified against the GitHub API per-asset `sha256` digest.
- **Notify-only updates** for BOTH the app and libjxl — never silent/automatic;
  the user opts in to every download. First-run libjxl install is still offered.
- Stack: **Wails v3 (beta.11), Go 1.27, Svelte + TypeScript, Vite.** Module path
  `github.com/dhcgn/jxleet` — regenerate the Wails bindings after backend changes.

## Core domain rules (get these right)
- **Three routes**, decided by input format × the active preset:
  - 🟢 **Transcode** — JPEG with `--lossless_jpeg=1`; reversible (djxl restores the
    exact original JPEG).
  - 🟠 **Reencode** — JPEG with `--lossless_jpeg=0`, or any JXL input; not reversible.
  - 🔵 **Encode** — PNG/APNG/GIF/EXR/NetPBM/PFM/PGX; lossless only at `-d 0`.
  - Route is **not** a property of the file — it follows from the preset. Unsupported
    files are **skipped and reported**, never abort the batch.
- **Presets** are YAML; `args` are passed to `cjxl` **verbatim** (no wrapper
  vocabulary). First-matching rule wins; trailing `"*"` is the fallback.
- **`replace` output policy** must follow the safe order: write temp → decode to
  verify readable (and byte-identical JPEG on the transcode route) → rename into
  place → move original to **recycle bin**. Never hard-delete; refuse replace where
  no recycle bin exists.
- **Distance is the single stored quality value**; quality (`-q`) is a display
  transform only.
- Each entry point (GUI / CLI / context menu) needs an explicit **preset binding**.
  First start creates `default-gui`, `default-cli`, and `default-explorer-context`
  as read-only presets and binds them; never silently replace an existing user
  binding.

## Working agreements
- Update `ARCHITECTURE.md` when structure or behavior changes — in the same change.
- The README introduces every view with a screenshot from `docs/screenshots/`
  (embedded as HTML `<img>`, not markdown). A change that adds or reworks a view
  refreshes its screenshot and the README in the same change.
- For visually oriented tasks, inspect the reference screenshots in `docs/screenshots/`.
  Capture the current view before development and the updated view afterward to
  validate the intended result.
- The README carries a **beta** notice — keep it until 1.0.
- Make surgical, complete changes; don't fix unrelated code.
- Only comment code that needs clarification.

## Build / run / test
```powershell
wails3 dev      # dev build, hot reload
wails3 build    # release build
task check      # build, vet, lint, race tests  (the CI gate)
```
- Requires **Go 1.27+** and `cjxl` on `PATH` for integration tests.
- Tests generate their own image fixtures at runtime — **do not commit binaries**.
- Prefer the smallest targeted test that covers the change; escalate to `task check`
  before finishing.

## CI
- `.github/workflows/build.yml`: `windows-latest`, Go 1.27 + Node, `task check`.
  The README build badge points at this.
- `.github/workflows/release.yml`: `v*` tags publish a GitHub Release (a tag
  like `v1.3.0-rc.1` becomes a pre-release); pushes to `dev` publish
  `vX.Y.Z-beta.N` pre-releases. Artifact: zip with `jxleet.exe` + `SHA256SUMS`.

## Releasing
- Releases are cut by pushing a version tag; **nothing is built locally**. The
  `/make-release` command walks through it — preflight, tag, watch, verify.
- Preflight: clean tree, on `main`, `main` not behind `origin/main`, `task check`
  green. Then `git tag -a vX.Y.Z -m "vX.Y.Z"` and push the tag.
- A hyphenated tag (`v1.3.0-rc.1`) publishes as a pre-release automatically.
  Betas (`vX.Y.Z-beta.N`) come from pushes to `dev`, never from tags.
- Watch the run to green (`gh run watch <id> --exit-status`) and verify with
  `gh release view <tag>`: `isPrerelease` must match the tag form and both assets
  must be present. Never move or re-push a published tag.
- **The release format is an updater contract — do not change any of these:**
  - Tag keeps the leading `v` (`vX.Y.Z`); the app strips it for the update check.
  - Asset stays `jxleet_<version-without-v>_windows_amd64.zip` — the updater's
    default matcher keys on the `windows`+`amd64` substrings.
  - The zip contains exactly one top-level entry (`jxleet.exe` at its root);
    multi-entry archives are rejected by the updater.
  - `SHA256SUMS` ships next to the zip; downloads are checksum-verified against it.
  - The version is stamped via `-X main.version` by `task build` with `VERSION`
    set (release.yml does this). Local builds report `dev` and never check for
    updates — that is intentional, not a bug.
  - Pre-releases stay excluded from the update feed (`/releases/latest`, no
    `CheckInterval`): stable users are never offered betas, and nothing downloads
    without the user asking (notify-only, locked decision above).

## Conventions
- Windows paths use backslashes. This machine has WSL, Docker, and `gh` available.
- Prefix shell commands covered by the **rtk** skill (`.agents/skills/rtk/SKILL.md`) with
  `rtk` — e.g. `rtk git status`, `rtk go test`, `rtk gh pr view` — so output is
  filtered before it reaches the agent's context. If `rtk --version` fails, run the raw
  command instead.
- No commit trailer.
