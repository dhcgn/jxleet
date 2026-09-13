# Architecture

<!-- jl:domain.glossary=Versioned project vocabulary in jl: source comments, generated into docs/GLOSSARY.md; normative for naming. -->

**jxleet** is a Windows front end for libjxl's `cjxl`. It encodes nothing itself:
it decides which files to hand over, assembles the argument list, runs the process,
verifies the result, and reports what came back. This document describes how the
system is built. `README.md` is the product specification; current scope and open
items are tracked in `AGENTS.md`.

## Glossary

User-visible names are fixed by the project glossary (`jl:` keys in
`docs/GLOSSARY.md`, chapters in `docs/GLOSSARY.topics.yaml`). Definitions live
as `jl:` comments in the source next to the code they describe; the markdown
is generated via `go generate ./...` (`internal/glossary`), and a test fails
the check gate when it is stale. Contributors and agents use a `jl:` key
whenever a defined term exists — e.g. every file takes one
`ref:jl:domain.route`, never a bare format name. A `ref:` with no matching
definition is written into a Broken references section and fails generation,
so dangling pointers cannot merge silently.

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
| `internal/app` | Wails services exposed to the frontend: file intake, conversion control, presets, toolchain status/install, app update check/install via the Wails updater (notify-only), history, collision resolution. Emits the `files`, conversion-progress, `conversion-file-start`, `collision-prompt` and `toolchain-progress` events |
| `internal/cli` | Strict path/flag parsing for path invocation, `--preset` override, exit codes. No dialogs |
| `internal/routes` | Route determination, route colours, effort-ladder reference data |
| `internal/preset` | YAML load/save, schema + version migration, validation, CRUD/import/export, entry-point bindings, read-only defaults |
| `internal/cjxl` | Command builder (args assembled verbatim from the preset) and process runner, output/JSON parsing. `RunWithStart` reports the child PID so in-flight files can be watched and cancelled individually |
| `internal/cjxl/flags` | `go:generate` scraper of `cjxl --help -v -v -v -v`: generated flag definitions, versioned snapshots (drive the Expert UI, preset validation, and the diff on version bump), parser tests against captured help output |
| `internal/djxl` | Decode verification for the replace safety order |
| `internal/jxlinfo` | `jxlinfo -v` invocation and output parsing (result and history drill-down) |
| `internal/process` | Child-process execution with hidden windows |
| `internal/convert` | The engine: queue, worker pool, pause/resume/cancel (whole-run and per-file via `CancelFile`), per-file start events (`OnFileStart` with PID), throughput-based ETA, collision handling, incremental adds after a finished run. `FileResult` carries the PID and resolved args so repeats of one file render as comparable rows |
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

`-e` 1–10, default 7. The effort ladder (coding tools × levels) is reference
data transcribed from libjxl's `doc/encode_effort.md` (spline detection is
verified in the libjxl source instead), independent of the installed cjxl
version. Tools with several strength stages are colour-graded
orange → yellow → green → blue, blue marking the strongest form; the
`8×8 blocks only` row is always yellow as it is a limitation. Every level
cell carries a tooltip, and rows can define separate stage lists for the
lossy and lossless modes. Tools of the other mode stay lit at half opacity.

## Presets

One YAML file per preset in `%APPDATA%\jxleet\presets\`:

- Schema: `name`, `description`, `version`, `output{policy, subfolder, on_collision, embed_settings, jxlinfo_sidecar}`, `rules[]{match[], args{}}`
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
all, rename, rename all, skip, skip all — rename re-prepares with the numbering
policy): the engine takes a `CollisionHandler` (nil keeps silent
skip), sticky answers short-circuit later prompts, and the service remembers a
*-all answer for the session (seeding later runs; a single answer clears it).
The service serializes
one outstanding prompt via the `collision-prompt` event with
`ResolveCollision`/`GetPendingCollision`.

When `output.embed_settings` is true, the per-file distance, effort and cjxl
version are inserted before `.jxl` (`photo.d1.00-e7-cjxl0.11.1.jxl`); distance
always keeps two decimals so suffixed files sort in numeric order, and the
transcode route reports `d0.00`. The GUI checkbox is a session-only override
(via `ConversionOptions`), like the output policy.

When `output.jxlinfo_sidecar` is true, the engine inspects the finalized output
and writes `<final>.jxlinfo.txt` next to it, always overwriting. The sidecar
runs after `Finalize`, so its failures only warn on the `FileResult` (surfaced
as `FileUpdate.warning`) and a failed conversion never leaves a sidecar behind.

## Concurrency and single instance

- Named pipe `\\.\pipe\jxleet-<user-sid>`; the first process becomes the owner
- Subsequent invocations connect, send their paths (+ `--preset`), receive an
  ack and exit within milliseconds — the calling application never waits
- The owner coalesces arriving batches into one run: one window, one progress
  bar. A handover arriving after the previous run finished auto-starts a new run
- Takeover if the pipe is stale (owner crashed): try-connect, then claim
- Engine: worker pool with processes and threads (`--num_threads`) configurable
  separately; pause/resume/cancel (whole-run plus one file at a time); run
  progress counts the run start as 10% so the bar moves immediately, then
  scales completed work over 10–100%; ETA from measured throughput over a sliding
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

`frontend/src` is split into three layers:

- **`App.svelte`** — the controller: grouped `$state` objects (`settings`,
  `meta`, `run`, `tools`, `history`, `cmdPreview` plus the intake and the
  session-only queue), deriveds, all actions and the Wails event wiring, and
  the shell (toolbar, preset strip, banners, statusbar, view switch).
- **`views/`** — one component per view (Main, Expert, Queue, Presets, Tools,
  History, Automatic, Stats). Each declares an explicit `Props` interface: state in
  via props, changes back via `onXxx` callback props; slider edits always go
  through callbacks because they must fire `onSettingsChanged()`. The two
  preset drafts in PresetsView are `$bindable` props. Single-consumer
  deriveds live in the view that renders them. The Main view is intake only:
  a flat file list in added order with route badges and counts, no grouping
  by action and no inline conversion. Main and Expert stage files via
  **Move to queue**: each listed file becomes one queue item
  (`ref:jl:domain.queue.item`) with a frozen copy of the run options and a
  snapshot label (distance with quality, effort, `+flags` hint), so later
  preset edits never touch staged items and the same file may be queued twice
  with different settings. Staging autostarts the run when idle; items staged
  mid-run wait for the next manual Start. The Queue view (`ref:jl:view.queue`) executes
  staged items back to back — one `cjxl` invocation per file with its frozen
  options — with global start/pause/cancel plus per-item cancel; every item spans
  two rows (data with status-only on top; PID with per-process CPU time and RAM
  plus the nowrap actions below), polled every
  second (`ref:jl:tech.tool.resources`, placeholder before the PID exists or
  after exit), and done rows show final size, ratio and needed time. Both rows
  are tinted per state (processing pulses gently; waiting, approval, unsupported,
  done, failed, cancelled and skipped each have their own static wash).
  Successes are recorded to History (with needed time); failed, cancelled and
  skipped rows stay in the Queue for retry and never reach History. Reclaim
  moves a row back to the Main intake and applies its snapshot to the session
  settings. Right-click menus are native Wails context menus
  registered in `main.go` (`file-table` → Clear via a `clear-table` event the
  frontend owns, `file-row` → per-file cancel with the input path as menu
  data, `queue-row` → remove/reclaim/show source/show JXL/open/clear done/
  clear all/cancel forwarded as `queue-*` events the frontend owns with the
  queue id as menu data). Closing with
  waiting or processing items warns once via `beforeunload` and discards them
  on confirm. Stats is a static mock (no backend
  calls) until wired to real run data.
- **`components/`** — reusable widgets (EffortLadder, QualitySliders,
  CommandPreview, JxlInfoPanel); **`lib/`** — pure modules (effort ladder
  data, quality/distance math, formatters, route and cjxl-flag helpers).

The active preset is the single source for the controls; Main and Expert
edits are session-only overrides with an amber warning and Revert. The
Expert view exposes the full generated flag surface with help tooltips, the
effort ladder, and the exact command preview. Native Wails file drops target
the whole window; a selected folder contributes only regular files directly
inside it. Dark by default, operable from 420 px. CSS stays one global sheet
(`public/style.css`); components reuse its classes.

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
  `-ldflags -X main.version` by `task build` with `VERSION` set
- App updates: the Wails built-in updater (`pkg/updater` + `providers/github`,
  initialized in `main.go` without a `CheckInterval`) checks `dhcgn/jxleet`
  releases with `ChecksumAsset: SHA256SUMS`. On start the GUI runs a silent
  `Check` (`GetAppUpdate`) and shows a dismissable banner when a newer release
  exists; the banner's Update button and the Tools view's manual check run
  `CheckAndInstall`, which opens the framework update window (release notes,
  progress, checksum verification, helper-mode binary swap on relaunch).
  Notify-only: dev builds, offline runs and pre-releases never warn, and
  nothing is ever downloaded without the user asking
