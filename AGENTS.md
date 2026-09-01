# AGENTS.md — jxleet

Guidance for AI agents and contributors working on this repository.

## What this is
**jxleet** is a Windows GUI/CLI front end for libjxl's `cjxl`. **It encodes nothing
itself** — it decides which files to hand over, assembles the arguments, runs the
process, verifies, and reports. Never add encoding logic; only orchestrate the
libjxl tools.

- `README.md` — product specification, with a screenshot per view.
- `ARCHITECTURE.md` — how the system is built. Read before structural work.

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

## Conventions
- Windows paths use backslashes. This machine has WSL, Docker, and `gh` available.
- Prefix shell commands covered by the **rtk** skill (`.agents/skills/rtk/SKILL.md`) with
  `rtk` — e.g. `rtk git status`, `rtk go test`, `rtk gh pr view` — so output is
  filtered before it reaches the agent's context. If `rtk --version` fails, run the raw
  command instead.
- No commit trailer.
