# Copilot instructions — jxleet

Guidance for AI agents and contributors working on this repository.

## What this is
**jxleet** (JPEG-XL-Expert-Encoding-Tool) is a Windows GUI/CLI front end for
libjxl's `cjxl`. **jxleet does not encode anything itself** — it decides which
files to hand to `cjxl`, assembles arguments, runs the process, and reports
results. Never add encoding logic; only orchestrate the libjxl tools.

## Project status: core complete through GUI
- Phases 0–8 are **done**. Latest: the Svelte/Wails GUI port — all eight mockup
  states, native file dialog/drop intake, preset/binding controls, route
  preview, asynchronous conversion events, pause/resume/cancel,
  toolchain install/status, and irreversible-replace confirmation. The visual
  port lives in `frontend/src/App.svelte` + `frontend/public/style.css`.
- Toolchain: `internal/toolchain` — official libjxl
  GitHub release lookup, per-asset sha256 verification, Deflate64-compatible
  Windows extraction, immutable versioned install + atomic current pointer,
  cjxl/djxl/jxlinfo status, notify-only update detection, and Expert flag
  lock/diff. `internal/app` exposes status and an explicitly user-triggered
  install method.
- Earlier IPC: `internal/ipc` — Windows named-pipe
  single-instance (per-user SID pipe), handover (secondary invocations send
  their paths + `--preset` and exit within ms), takeover when no owner is
  reachable, and coalescing of multiple invocations. `main.go` now does
  single-instance startup and feeds handed-over paths into the app service +
  a `files` Wails event. The engine was validated on the real committed
  `test-data/` sample (JPEG→transcode with byte-identical reconstruction,
  JXL→reencode, 8/16-bit PNG→encode). `task check` is green.
- Still open nearby: jxlinfo metadata parsing, persistent logs, and the
  effort-ladder source data. Phase 9 CLI/context-menu work is implemented:
  strict path invocation, `--preset` overrides, per-user Explorer registration,
  and registry cleanup. Next per the plan: **Phase 10 — polish**.
- Authoritative documents (read these first):
  - `README.md` — product specification.
  - `FEATURES.md` — scope checklist (**living**, see rules below).
  - `develop-time/IMPLEMENTATION_PLAN.md` — architecture + phased plan. **Start here.**
  - `develop-time/design/jxlconv-mockups.html` — the 8 GUI states (reference only).

## Locked decisions (do not silently change)
- **Windows 10/11 x64 only** for v1. Strip/ignore darwin/linux/ios/android build
  targets. Core features are Windows-specific (recycle bin, Explorer context menu,
  named-pipe handover, `%APPDATA%`/`%LOCALAPPDATA%`).
- **Module path** is `github.com/dhcgn/jxleet` (the template's `changeme` must be
  renamed; regenerate Wails bindings after).
- **Managed toolchain** = libjxl GitHub release asset `jxl-x64-windows-static.zip`,
  integrity-verified against the GitHub API per-asset `sha256` digest.
- **Notify-only updates** for BOTH the app and libjxl — never silent/automatic;
  the user opts in to every download. First-run libjxl install is still offered.
- Stack: **Wails v3 (beta.11), Go 1.27, Svelte + TypeScript, Vite.**

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
  First start creates and binds the read-only `Default` preset to all three;
  never silently replace an existing user binding.

## Working agreements
- **`FEATURES.md` is the source of truth for scope.** When you implement a listed
  item, tick `[ ] → [x]` in the same change. Add newly discovered scope; reword
  items that diverge. Do not implement a feature without updating `FEATURES.md`.
- Keep `develop-time/IMPLEMENTATION_PLAN.md` in sync when architecture changes.
- Make surgical, complete changes; don't fix unrelated code.
- Only comment code that needs clarification.

## Architecture (target layout — see plan §1)
Thin `main.go`; pure testable domain in `internal/{routes,preset,cjxl,cjxl/flags}`;
Windows edges isolated in `internal/{output/recyclebin,ipc,shellext}`; engine in
`internal/convert`; toolchain in `internal/toolchain`. Frontend Svelte views map
1:1 to the 8 mockup states (plan §9); the mockup HTML is **ported** to components,
not shipped (plan §9.1).

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

## CI/CD (to be added — plan §10a, Phase 0)
- `.github/workflows/ci.yml`: `windows-latest`, Go 1.27 + Node, run `task check`.
  The README build badge points at this.
- `.github/workflows/release.yml` on `v*` tags: `wails3 package` → zip →
  `SHA256SUMS` → GitHub Release.

## Conventions
- Windows paths use backslashes. This machine has WSL, Docker, and `gh` available.
- No Commit trailer
- Follow the phase order in `IMPLEMENTATION_PLAN.md` §11 (CI first, then scaffold).
