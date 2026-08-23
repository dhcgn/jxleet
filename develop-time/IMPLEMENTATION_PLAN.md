# jxleet — Implementation Plan & Architecture

Status: design phase. This document turns `README.md` (product spec) and
`FEATURES.md` (checklist) into a buildable plan. The GUI states are defined in
`develop-time/design/jxlconv-mockups.html` (8 states: Drop, Basic, Expert,
Running, Done, Tools, Automatic, Presets).

---

## 0. Decisions locked for v1

| Decision | Choice |
|---|---|
| Platform | **Windows 10/11 x64 only.** Strip darwin/linux/ios/android build targets. |
| Module path | Rename `changeme` → `github.com/dhcgn/jxleet`, regenerate Wails bindings, delete demo code (`GreetService`, `time` event, template `App.svelte`). |
| Toolchain source | Official **libjxl GitHub releases**, asset `jxl-x64-windows-static.zip` (static, no DLLs). |
| Checksum | Verify download against the GitHub API per-asset `digest` (`sha256:…`). No separate checksums file exists. |
| Stack | Wails v3 (beta.11), Go 1.27, Svelte + TypeScript, Vite. |
| Updates | **Notify-only** for both the app and libjxl — never silent/auto. User opts in to any download. |

### FEATURES.md is the living checklist
`FEATURES.md` is treated as the **source of truth for scope and progress** and is
kept in sync throughout implementation:
- Every phase that completes a listed item **ticks its `[ ]` → `[x]`** in the same
  change/PR.
- New scope discovered during implementation is **added** to FEATURES.md (and, if
  architectural, reflected here in the plan).
- If behaviour diverges from a listed item, the item is **reworded** rather than
  left stale. CI/PR review should reject changes that implement a feature without
  updating FEATURES.md.

### Confirmed toolchain facts (verified against libjxl v0.12.0)
- Latest release resolvable via `GET /repos/libjxl/libjxl/releases/latest`.
- Windows x64 static asset present and self-contained.
- Each asset carries `digest: "sha256:<hex>"` → our integrity source of truth.
- **Zip layout confirmed (v0.12.0):** binaries live at
  `x64-windows-static/bin/{cjxl,djxl,jxlinfo}.exe` inside the zip.

---

## 1. Package / project layout

```
main.go                     wiring only: parse args → decide GUI vs handover → app.Run
internal/
  app/                      Wails service(s) exposed to the frontend (bindings root)
  routes/                   route determination (Transcode/Reencode/Encode) + colours
  preset/                   YAML load/save, schema+version+migration, validation, bindings
  cjxl/                     command builder, process runner, output/JSON parsing
    flags/                  go:generate scraper of `cjxl --help -v -v -v -v` → flags.go
  toolchain/                version query, download, sha256 verify, atomic install, diff
  convert/                  the engine: queue, worker pool, throughput/ETA, pause/cancel
  output/                   policies (alongside/subfolder/replace), collision, recyclebin
  ipc/                      single-instance named-pipe owner + handover client, coalescing
  cli/                      path-invocation mode, --preset override, exit codes
  shellext/                 per-user Explorer context-menu register/unregister (registry)
  config/                   %APPDATA% paths, config.yaml (three entry-point bindings)
  logging/                  run logs under %LOCALAPPDATA%\jxleet\logs\
frontend/                   Svelte views mapped to the 8 mockup states
build/windows/             Wails windows build/package taskfile (keep only this OS)
```

Rationale: a thin `main`, a pure-domain core (`routes`, `preset`, `cjxl/flags`)
that is unit-testable without a GUI, and Windows-specific edges
(`output/recyclebin`, `ipc`, `shellext`) isolated behind small interfaces.

---

## 2. Core domain

### 2.1 Route determination (`internal/routes`)
Route = input format × preset rule (**not** a property of the file).

| Route | Input | cjxl behaviour | Reversible | Colour |
|---|---|---|---|---|
| 🟢 Transcode | JPEG | `--lossless_jpeg=1` (repack) | yes (djxl → identical JPEG) | green |
| 🟠 Reencode | JPEG (`--lossless_jpeg=0`), JXL | decode + re-encode | no | orange |
| 🔵 Encode | PNG, APNG, GIF, EXR, NetPBM, PFM, PGX | from pixels; lossless only at `-d 0` | at d0 only | blue |

Rules:
- JPEG → Transcode or Reencode depending on the effective `--lossless_jpeg`.
- JXL input → always Reencode.
- Everything else → Encode.
- Format detection by content (magic bytes) with extension as a hint; unsupported
  files are **skipped and reported**, never abort the batch.

### 2.2 Distance / Quality (`-d` ↔ `-q`)
Single stored quantity = **distance**. Quality is a display transform only, using
cjxl's own mapping (`-q 100→d0`, `q≥30: d=0.1+(100-q)*0.09`, `q<30` quadratic —
mirror libjxl's `JxlEncoderDistanceFromQuality`). Toggle changes display, never
the stored value. Distance range in UI: 0–15 (slider ×10 → 0.0–1.5+ region).

### 2.3 Effort (`-e` 1–10)
Default 7 (“squirrel”). The **effort ladder** (grid of coding tools × levels) is
authored, maintained data (`internal/routes` or a small `effort.go` table),
independent of installed version, showing which tools activate per level and
which are struck through for the current mode.

---

## 3. Presets (`internal/preset`)

- One YAML file per preset in `%APPDATA%\jxleet\presets\`.
- Schema: `name`, `description`, `version`, `output{policy,subfolder,on_collision}`,
  `rules[]{match[], args{}}`. First-match-wins; trailing `"*"` = fallback.
- `args` passed to cjxl **verbatim** (short/long forms both valid; valueless flags
  as `true`). No wrapper vocabulary.
- **Validation** against `cjxl -v -v --help` of the *installed* version before a
  run; unknown flag → non-zero exit, run refused.
- Version + migration path; import does **not** adopt the source output policy
  (imports default to `alongside`); collision handling on import.
- CRUD: create, duplicate, rename, delete, export, import.

**Entry-point bindings** live in `config.yaml` — one preset name each for
`gui`, `cli`, `contextmenu`. On first start all three are bound to the read-only
entry-point default presets. Existing user bindings are preserved; the app refuses to run
only when a binding is missing or points to an unavailable preset.

---

## 4. Output policies (`internal/output`)

| Policy | Behaviour |
|---|---|
| alongside | result next to original (default) |
| subfolder | `./<subfolder>/` (default `jxl`) |
| replace | original → recycle bin, after verification |

`replace` fixed, non-short-circuitable order:
1. write temp file in target dir,
2. **decode it** with djxl to prove readable; on Transcode route also confirm the
   reconstructed JPEG is **byte-identical**,
3. rename into place,
4. only then move original to **recycle bin** (Win32 `SHFileOperation`
   `FO_DELETE|FOF_ALLOWUNDO`, via `go-ole`/syscall).
Any failure → original untouched. No recycle bin available (network/removable) →
refuse to replace, never hard-delete. Deletion rule selectable per route;
separate confirmation for irreversible routes (names the file count).

---

## 5. Toolchain manager (`internal/toolchain`)

- Show installed versions of `cjxl/djxl` (`--version`) and the bundled
  `jxlinfo` release version on every start (`jxlinfo` itself has no version-only
  flag in libjxl v0.12.0).
- Query latest via GitHub API; compare.
- Download `jxl-x64-windows-static.zip`, verify sha256 against the GitHub API
  per-asset `digest`, extract to a temp dir, then **atomic install** (extract to
  a unique staging directory → verify exes run → move the three exes into the
  immutable `versions\<version>\bin` directory → atomically replace
  `current.txt`). Binaries live under `%LOCALAPPDATA%\jxleet\bin\`.
- The v0.12.0 official ZIP uses **Deflate64**, which Go's `archive/zip` cannot
  decompress. Go performs archive preflight (contained paths, no symlinks, size
  limits), then uses the Windows 10/11 built-in `Shell.Application` extractor;
  no third-party extractor is required.
- First-run: detect missing tools, offer install (README first-run fetch).
- **Updates are notify-only** — never automatic/background. When a newer libjxl
  release exists the app shows a notice; the user explicitly triggers the
  download (which then verifies sha256 + atomically installs). Same policy as the
  app itself (§10b).
- On version mismatch: **lock expert flags** and show a **flag diff** (see §6).
- `internal/app` exposes `GetToolchainStatus` and
  `InstallLatestToolchain`; status only notifies, while installation is
  explicitly user-triggered. The first-run offer belongs to the Tools GUI view.

---

## 6. Flag sync (`internal/cjxl/flags`, `go:generate`)

- `//go:generate` runs a small generator that executes
  `cjxl --help -v -v -v -v`, parses the flag list (name, short/long, takes-value,
  mode applicability), and emits `flags_generated.go` + a versioned JSON snapshot.
- The JSON snapshot enables the **diff on version bump** and drives Expert-mode UI
  + preset validation.
- Parser is brittle by nature → keep a captured sample of the help output in
  `testdata/` and unit-test the parser against it.

---

## 7. Concurrency & single instance (`internal/ipc`, `internal/convert`)

- **Named pipe** `\\.\pipe\jxleet-<user-sid>`; first process becomes owner.
- Subsequent invocations connect, send their paths (length-framed JSON), get an
  ack, and **exit within ms** — Lightroom is never left waiting.
- Owner **coalesces** arriving batches into one run: one window, one progress bar,
  with a note showing how many invocations were merged.
- **Takeover** if the pipe is stale/unreachable (owner crashed): try-connect →
  on failure, claim ownership.
- Engine: worker pool; **processes and threads configurable separately**
  (N parallel cjxl processes × `--num_threads`); pause/cancel; ETA from measured
  throughput over recent files (sliding window), not a fixed guess.

---

## 8. Entry points

1. **GUI** (`internal/app` + Svelte): drag/drop + file dialog, direct folder files only,
   Basic/Expert split, live command preview, results with jxlinfo figures.
2. **CLI** (`internal/cli`): any mix of file/folder paths → uses `cli` binding,
   no dialog; `--preset <name>` overrides for one call; unknown preset → non-zero
   exit (no fallback). Paths present ⇒ handover path (§7).
3. **Context menu** (`internal/shellext`): per-user registry keys (no admin) for
   file / folder / folder-background; menu text carries the preset name
   (“To JXL — <preset>”); register/unregister from within the app. Windows 11:
   entry under “Show more options”.

Phase 9 implementation uses the stdlib-only parser in `internal/cli` for strict
path/flag handling. A first invocation with paths starts the same asynchronous
engine without a settings prompt; later invocations hand over paths and an
explicit preset through `internal/ipc`.

---

## 9. Frontend (Svelte) — views ↔ mockup states

| View | Mockup | Key elements |
|---|---|---|
| Drop | v1 | dropzone, preset select, Tools button, route counts |
| Basic | v2 | route summary cards, file table w/ route badges, compression, JPEG-handling, output, Convert button (dominant-route colour) |
| Expert | v3 | path-mode seg, effort ladder+grid, cmd preview, distance/quality toggle, all flags |
| Running | v4 | progress, ETA, throughput, queue, cancel/pause |
| Done | v5 | size balance per file/total, jxlinfo figures, failures |
| Tools | v6 | versions, mismatch, download/update |
| Automatic | v7 | compact window for coalesced auto-invocation |
| Presets | v8 | library + YAML editor, bindings per entry point |

Dark by default; operable from 420 px. Route colours follow the setting
everywhere (summary, rows, Convert button).

### 9.1 Porting the mockup (`jxlconv-mockups.html` → Svelte)
The mockup is a **static, single-file** design reference: 8 states switched by a
tab bar, inline CSS, and vanilla JS. It is **not** Svelte/Wails-compatible and
will not be dropped in as-is. Porting plan:
- Extract the CSS custom properties / colour tokens (route colours, panels) into a
  shared theme (`app.css` / a Svelte store), keep them as the single source.
- Rebuild each state as a Svelte component (one per view in §9); replace the tab
  switcher with real routing/app state.
- Replace mock `data-testid` values + hard-coded numbers with data from Wails
  bindings and events (route counts, file table, cmd preview, progress).
- Preserve `data-testid` attributes on interactive elements to enable frontend
  E2E tests.
- Keep the mockup file in `develop-time/design/` as reference; it is not shipped.
- **Current implementation:** the eight views are explicit branches in
  `frontend/src/App.svelte`, with the shared visual system in
  `frontend/public/style.css`; they are wired to native dialog intake, Wails
  bindings/events, real preset previews, asynchronous engine progress, and
  toolchain status/install. The shell fills the Wails client area and leaves
  minimize/maximize/close controls to the native Wails host. Native Wails file
  drops target the drop zone, Open File/Open Folder are separate multi-select
  actions, folders enumerate only direct regular files, and conversion actions
  remain visible while content scrolls. Storage rows open their corresponding
  directories in Windows Explorer. File/result tables use fixed, non-overlapping
  columns with compact path display and native full-path tooltips; the effort
  matrix uses compact accessible capability icons; the Presets table displays
  core distance/quality, effort, and JPEG mode summaries with `Mixed` for
  differing rules. Split into child components only when the view logic
  materially grows.

---

## 10. Testing

- Pure domain (`routes`, `preset`, `cjxl` builder, `flags` parser, d↔q,
  throughput/ETA): table-driven unit tests, no GUI, no cjxl.
- Integration tests require `cjxl` on PATH; generate image fixtures at runtime
  (no binaries committed). Cover each route + replace-verification path.
- `task check` = build, vet, lint, race tests.

---

## 10a. CI/CD & Release automation (GitHub Actions)

Implemented in `.github/`; README badges reference the build workflow and Go
Report Card:

- **CI (`build.yml`)** on push/PR: `windows-latest`, set up Go 1.27 + Node, install
  `wails3` + `task`, run `task check` (build, vet, lint, race tests). Cache Go
  modules and npm. This is what the build badge points at.
- **Lint:** `golangci-lint` (Go) + `svelte-check`/`tsc` (frontend).
- **Release (`release.yml`)** on tag `v*`: `wails3 package` for windows/amd64,
  zip the artifact, generate a `SHA256SUMS`, and publish a GitHub Release with the
  assets (see `go-release` conventions: semver tags, changelog, GoReleaser
  optional for the CLI/binary side).
- **Dependabot/Renovate** for Go + npm + Actions.
- Embed version info at build (`-ldflags -X`) so the app can report its own
  version and compare against the latest release (feeds §10b).

## 10b. Application self-update (decision required)

Distinct from the §5 *toolchain* manager (which updates libjxl, not jxleet).

- **Current README stance:** the **app** is updated **manually** ("Download the
  latest release, unzip, run") — i.e. no self-update by design.
- **Wails v3 (beta.11)** ships **no official auto-updater**. Options:
  1. **Manual (README default)** — do nothing; just surface "a newer jxleet
     release exists" via the GitHub API and link to the download. *(lowest risk,
     recommended for v1)*
  2. **Assisted self-update** — reuse the §7 atomic-swap pattern: download the new
     zip, verify sha256, stage, and replace `jxleet.exe` on next launch.
     Windows can't overwrite a running exe → needs a small relauncher/helper.
  3. **Third-party updater** (e.g. an external framework) — extra dependency.
- **Decision (locked): notify-only for v1.** No automatic/background update of
  the app — surface "a newer jxleet release exists" via the GitHub API and link
  to the download. Consistent with the libjxl toolchain policy (§5): the app
  notifies about both its own and libjxl updates, and never applies them silently.
  Assisted atomic self-update (option 2 above) is deferred to post-v1.

## 11. Milestones (mapped to FEATURES.md)

0. **CI first** — add `.github/workflows/build.yml` (Windows, `task check`) so the
   build badge is real from commit one; Dependabot; lint config. *(infra)*
1. **Scaffold** — module rename, delete template, package skeleton, config paths,
   Windows-only build. *(no user-facing feature yet)*
2. **cjxl core** — flags generator, command builder, runner, route determination,
   d↔q, effort table. *(Conversion group)*
3. **Presets** — schema/validation/CRUD/import-export, bindings. *(Presets group)*
4. **Output** — policies + recycle-bin replace w/ verification. *(Output group)*
5. **Engine** — queue, worker pool, pause/cancel, throughput/ETA. *(Concurrency)*
6. **IPC** — single instance, handover, coalescing, takeover. *(Concurrency)*
7. **Toolchain** — versions, download+verify, atomic update, diff/lock. *(Toolchain)*
8. **GUI** — the 8 views. *(Interface — implemented)*
9. **CLI + context menu** — path invocation, registry integration. *(Entry points — implemented)*
8b. **GUI port** — convert `jxlconv-mockups.html` into Svelte state views +
    shared theme, wired to bindings (see §9.1). *(Interface — implemented)*
10. **Polish** — logs view, size balance, jxlinfo figures.
11. **Release automation** — `release.yml` on tag: `wails3 package`, zip,
    SHA256SUMS, GitHub Release; app "newer version available" notice (§10a/§10b).

---

## 12. Remaining open questions (non-blocking, resolve as reached)

- Effort-ladder tool/level matrix remains authored reference data until an
  authoritative libjxl source mapping is available.
- Detailed jxlinfo parsing and persistent log-view UX remain Phase 10 polish.
