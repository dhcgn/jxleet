# Architecture

**jxleet** is a Windows front end for libjxl's `cjxl`. It encodes nothing itself:
it decides which files to hand over, assembles the argument list, runs the process,
verifies the result, and reports what came back. This document describes how the
system is built. `README.md` is the product specification; current scope and open
items are tracked in `AGENTS.md`.

## Stack

- **Wails v3 (beta.11)** — Go backend, WebView frontend, native window
- **Go 1.27**, **Svelte + TypeScript**, **Vite**
- **Windows 10/11 x64 only.** Core features are Windows-specific: recycle bin,
  Explorer context menu, named pipes, `%APPDATA%`/`%LOCALAPPDATA%`
- No encoding logic anywhere in this repository — only orchestration of `cjxl`,
  `djxl` and `jxlinfo` from libjxl

## System flow

```
 GUI (Wails window) ─┐
 CLI (paths) ────────┼─► internal/ipc      named pipe, per-user SID
 Explorer menu ──────┘      │              single instance + handover:
                            │              later invocations send paths
                            │              (+ --preset), get an ack, exit
                            ▼
                     internal/app           Wails service layer
                            │              bindings + events
                            ▼
                     internal/convert       the engine: queue, worker
                            │              pool, pause/cancel, ETA
                            ▼
                     internal/routes        route = format × preset
                            │
          ┌─────────────────┼──────────────────┐
          ▼                 ▼                  ▼
    internal/cjxl     internal/djxl      internal/jxlinfo
    encode           verify decode      inspect metadata
          │                 │                  │
          └──── internal/process (hidden child processes) ────┘
                            │
                            ▼
                     internal/output       alongside · subfolder · replace
                            │              (recycle bin, verified)
                            ▼
                     internal/history      append-only JSONL
```

## Packages

| Package | Responsibility |
|---|---|
| `main.go` | Thin wiring: parse args, single-instance startup, launch the app |
| `internal/app` | Wails services exposed to the frontend: file intake, conversion control, presets, toolchain status/install, history, collision resolution. Emits the `files`, conversion-progress, `collision-prompt` and `toolchain-progress` events |
| `internal/cli` | Strict path/flag parsing for path invocation, `--preset` override, exit codes. No dialogs |
| `internal/routes` | Route determination, route colours, effort-ladder reference data |
| `internal/preset` | YAML load/save, schema + version migration, validation, CRUD/import/export, entry-point bindings, read-only defaults |
| `internal/cjxl` | Command builder (args assembled verbatim from the preset) and process runner, output/JSON parsing |
| `internal/cjxl/flags` | `go:generate` scraper of `cjxl --help -v -v -v -v`: generated flag definitions, versioned snapshots (drive the Expert UI, preset validation, and the diff on version bump), parser tests against captured help output |
| `internal/djxl` | Decode verification for the replace safety order |
| `internal/jxlinfo` | `jxlinfo -v` invocation and output parsing (result and history drill-down) |
| `internal/process` | Child-process execution with hidden windows |
| `internal/convert` | The engine: queue, worker pool, pause/resume/cancel, throughput-based ETA, collision handling, incremental adds after a finished run |
| `internal/output` | Output policies (alongside/subfolder/replace), name collisions, recycle bin |
| `internal/ipc` | Named-pipe single instance (per-user SID), handover, takeover when the owner is unreachable, coalescing |
| `internal/shellext` | Per-user Explorer context-menu registration (registry, no admin) |
| `internal/toolchain` | libjxl release lookup, download, sha256 verification, extraction, atomic versioned install |
| `internal/config` | `%APPDATA%` paths, `config.yaml` with the three entry-point bindings |
| `internal/history` | Append-only, torn-write-tolerant JSONL history |
| `frontend/` | Svelte app. All views are branches in `frontend/src/App.svelte`; the visual system lives in `frontend/public/style.css`. Wails bindings are generated into `frontend/bindings/github.com/dhcgn/jxleet/internal/app/` |

## Domain model

### Routes

Route = input format × active preset rule — never a property of the file alone.

| Route | Input | cjxl behaviour | Reversible |
|---|---|---|---|
| 🟢 Transcode | JPEG + `--lossless_jpeg=1` | repack losslessly | yes — djxl restores the original JPEG byte for byte |
| 🟠 Reencode | JPEG + `--lossless_jpeg=0`, or any JXL | decode + re-encode | no |
| 🔵 Encode | PNG, APNG, GIF, EXR, NetPBM, PFM, PGX | encode from pixels | only at `-d 0` |

Format detection is by content (magic bytes) with the extension as a hint.
Unsupported files are skipped and reported, never abort the batch.

### Distance and quality

Distance (`-d`) is the single stored quality value. Quality (`-q`) is a display
transform using cjxl's own mapping; toggling the display never changes the stored
value.

### Effort

`-e` 1–10, default 7. The effort ladder (coding tools × levels) is authored
reference data, independent of the installed cjxl version.

## Presets

One YAML file per preset in `%APPDATA%\jxleet\presets\`:

- Schema: `name`, `description`, `version`, `output{policy, subfolder, on_collision}`, `rules[]{match[], args{}}`
- Rules evaluate top to bottom, first match wins; a trailing `"*"` rule is the fallback; without one, unmatched files are skipped and reported
- `args` are passed to `cjxl` **verbatim** — short and long forms both valid, valueless flags as `true`. No wrapper vocabulary
- Presets carry a `# yaml-language-server` modeline pointing at the committed `preset.schema.json`
- Keys are validated against the **installed** `cjxl` help before a run; an unknown flag refuses the run
- Import never adopts the source's output policy (always `alongside`)

Entry-point bindings (GUI, CLI, context menu) live in `config.yaml`. First start
creates the read-only `default-gui`, `default-cli` and `default-explorer-context`
presets and binds them; existing user bindings are never silently replaced.

## Output and safety

| Policy | Behaviour |
|---|---|
| `alongside` | result next to the original (default) |
| `subfolder` | `./<subfolder>/` relative to the source |
| `replace` | result takes the original's place; original to the recycle bin |

`replace` follows a fixed, non-short-circuitable order:

1. write a temp file in the target directory
2. decode it with djxl to prove it readable — on the transcode route also confirm
   the reconstructed JPEG is byte-identical
3. rename into place
4. only then move the original to the **recycle bin**

Any failure leaves the original untouched. There is no hard delete, ever; where
no recycle bin exists (network shares, some removable media) replace is refused.

Name collisions follow the preset's `on_collision` (`skip` / `number` /
`overwrite`). Under `skip` the GUI prompts per collision (overwrite, overwrite
all, skip, skip all): the engine takes a `CollisionHandler` (nil keeps silent
skip), sticky answers short-circuit later prompts, and the service serializes
one outstanding prompt via the `collision-prompt` event with
`ResolveCollision`/`GetPendingCollision`.

## Concurrency and single instance

- Named pipe `\\.\pipe\jxleet-<user-sid>`; the first process becomes the owner
- Subsequent invocations connect, send their paths (+ `--preset`), receive an
  ack and exit within milliseconds — the calling application never waits
- The owner coalesces arriving batches into one run: one window, one progress
  bar. A handover arriving after the previous run finished auto-starts a new run
- Takeover if the pipe is stale (owner crashed): try-connect, then claim
- Engine: worker pool with processes and threads (`--num_threads`) configurable
  separately; pause/resume/cancel; ETA from measured throughput over a sliding
  window of recent files

## Toolchain management

`internal/toolchain` manages the libjxl binaries under `%LOCALAPPDATA%\jxleet\bin\`:

- Latest release and per-asset `sha256` digests from the GitHub API; asset
  `jxl-x64-windows-static.zip`
- The official zip uses Deflate64, which Go's `archive/zip` cannot read: Go
  performs a preflight (contained paths, no symlinks, size limits) then extracts
  via the Windows `Shell.Application` — no third-party extractor
- Install is atomic: unique staging dir → verify the exes run → immutable
  `versions\<version>\bin` → atomically replace the current pointer
- Updates are **notify-only** for both the app and libjxl — the user triggers
  every download; the first-run install is offered
- On version mismatch the Expert flags are locked and the flag snapshot diff is
  shown

## Frontend

The eight views (Drop, Basic/Main, Expert, Running, Done, Tools, Automatic,
Presets — Running and Done are inline states of Main) are explicit branches in
`App.svelte`. The active preset is the single source for the controls; Main and
Expert edits are session-only overrides with an amber warning and Revert. The
Expert view exposes the full generated flag surface with help tooltips, the
effort ladder, and the exact command preview. Native Wails file drops target the
whole window; a selected folder contributes only regular files directly inside
it. Dark by default, operable from 420 px.

## Storage

```
%APPDATA%\jxleet\config.yaml       settings and the three entry-point bindings
%APPDATA%\jxleet\presets\          one YAML file per preset
%APPDATA%\jxleet\history.jsonl     one JSON line per successful conversion
%LOCALAPPDATA%\jxleet\bin\         the managed libjxl binaries
%LOCALAPPDATA%\jxleet\logs\        run logs
```

## Testing and CI

- Pure domain packages are unit-tested without a GUI or cjxl
- Integration tests require `cjxl` on `PATH` and generate their own image
  fixtures at runtime — no binaries in the repository
- `task check` = build, vet, lint, race tests
- CI: `.github/workflows/build.yml` — `windows-latest`, Go 1.27 + Node,
  `task check`; every run uploads `bin/jxleet.exe` as a workflow artifact
  (also when later steps fail)
- Releases: `.github/workflows/release.yml` — on `v*` tags (a `-rc.N`/`-beta.N`
  suffix marks a pre-release) and on every `dev` push (`vX.Y.Z-beta.N`
  pre-releases). Publishes a zip with `jxleet.exe` and a `SHA256SUMS` file to
  a GitHub Release; the version is stamped into the binary via
  `-ldflags -X main.version` by `task build` with `VERSION` set.
