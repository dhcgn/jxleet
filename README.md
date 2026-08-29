<div align="center">

# jxleet

**JPEG-XL-Expert-Encoding-Tool** — a comfortable way to use `cjxl` on Windows.

[![Release](https://img.shields.io/github/v/release/dhcgn/jxleet?logo=github)](https://github.com/dhcgn/jxleet/releases/latest)
[![Build](https://github.com/dhcgn/jxleet/actions/workflows/build.yml/badge.svg)](https://github.com/dhcgn/jxleet/actions/workflows/build.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dhcgn/jxleet)](https://goreportcard.com/report/github.com/dhcgn/jxleet)
[![Go Reference](https://pkg.go.dev/badge/github.com/dhcgn/jxleet.svg)](https://pkg.go.dev/github.com/dhcgn/jxleet)
[![Downloads](https://img.shields.io/github/downloads/dhcgn/jxleet/total?logo=github)](https://github.com/dhcgn/jxleet/releases)
[![License](https://img.shields.io/github/license/dhcgn/jxleet)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%2010%2F11-0078D4?logo=windows)](#requirements)

*TODO: Screenshot of the main window with a mixed drop, showing the three route counts*

</div>

---

> ### jxleet does not encode anything
>
> Every byte of every `.jxl` file this tool produces is written by **`cjxl`** from the
> [libjxl](https://github.com/libjxl/libjxl) project. The format, the encoder, the codec
> research and all of the genuinely hard work are theirs. jxleet is a front end: it decides
> which files to hand over, assembles the arguments, runs the process, and shows you what
> came back.
>
> If you are comfortable on the command line, use `cjxl` directly — you lose nothing.
> jxleet exists for the parts that get tedious at scale: batching, keeping the binaries
> current, plugging into Lightroom, and not having to remember which of your files are
> still recoverable.

## Why this exists

Most converters ask for a quality slider and a folder. There is a more useful question to
ask first: **is this conversion reversible?**

Hand `cjxl` a JPEG and it will, by default, repack it losslessly — `djxl` can reconstruct
the original JPEG byte for byte, and you have saved about 20 %. Pass `--lossless_jpeg=0`
and it decodes and re-encodes instead, giving you something much smaller that can never be
turned back. Those are entirely different operations behind one flag.

jxleet treats them as different **routes**, colours them differently, counts them
separately, and will not delete an original until it has decoded the result and proven it
readable.

## Contents

- [jxleet](#jxleet)
  - [Why this exists](#why-this-exists)
  - [Contents](#contents)
  - [The three routes](#the-three-routes)
  - [Install](#install)
    - [Requirements](#requirements)
  - [Three ways to run it](#three-ways-to-run-it)
    - [1. The window](#1-the-window)
    - [2. The command line](#2-the-command-line)
    - [3. The Explorer context menu](#3-the-explorer-context-menu)
  - [Presets](#presets)
    - [A second example](#a-second-example)
    - [Validation](#validation)
    - [Sharing](#sharing)
  - [The Lightroom workflow (my personal use case)](#the-lightroom-workflow-my-personal-use-case)
  - [Distance, quality and effort](#distance-quality-and-effort)
  - [Output policies](#output-policies)
  - [The managed toolchain](#the-managed-toolchain)
  - [Supported formats](#supported-formats)
  - [Where things are stored](#where-things-are-stored)
  - [Building from source](#building-from-source)
  - [FAQ](#faq)
  - [License](#license)

## The three routes

Every file takes one of three routes. The route follows from the input format **times your
preset** — a JPEG can take either of the first two, and which one it takes is your decision,
not a property of the file.

| Route | Input | What happens | Reversible |
|---|---|---|---|
| 🟢 **Transcode** | JPEG | Repacked losslessly into a JXL container | **Yes** — `djxl` restores the original JPEG byte for byte |
| 🟠 **Reencode** | JPEG, JXL | Decoded and encoded again with your settings | No |
| 🔵 **Encode** | PNG, GIF, EXR, NetPBM, PFM, PGX | Encoded from pixels, lossless at distance 0 | Only at distance 0 |

The colours are used everywhere: on the drop summary, on every row of the file list, and on
the convert button itself, which takes the colour of whichever route dominates your batch.
Change the rule that governs JPEG and the counts, badges and size estimates all move at
once.

*TODO: Screenshot of the file list with mixed route badges*

## Install

Download the latest release, unzip, run `jxleet.exe`. No installer, nothing written outside
your user profile.

On first start jxleet notices that the libjxl tools are missing and offers to fetch them.
Accept, pick a preset for each of the three entry points, and you are ready.

### Requirements

Windows 10 or 11, 64-bit. Nothing else — the libjxl binaries are downloaded and managed by
the application.

## Three ways to run it

**Each of the three requires a preset.** Not a default that jxleet picks for you — one you
choose, once, per entry point. A tool that can replace files should never guess at your
intent. On first start the read-only `default-gui`, `default-cli`, and
`default-explorer-context` presets are bound to their respective entry points;
jxleet refuses to run only when a binding is missing or invalid.

*TODO: Screenshot of the preset bindings panel with the three entry points*

### 1. The window

Drag files or folders in - native Windows drag-and-drop accepts
files or folders in every view, and the toolbar keeps separate **Open File** and **Open Folder**
actions always available. A selected folder contributes only regular files directly
inside it; subfolders are not traversed, and anything unsupported is skipped rather than
aborting the batch. If no preset is selected, the files stay in the queue and the window
explains that a preset is needed for route classification.

Settings split into **Main** - the essentials - and **Expert**, which exposes the full
generated `cjxl` flag surface; every flag includes its help text as a tooltip. The selected
preset seeds the controls: switching presets re-resolves everything immediately, and the
preset strip under the toolbar spells out each rule - the catch-all and every format with
its own settings. Edits in Main/Expert are session-only overrides; the strip then warns
in amber ("settings differ — preset not in effect") and a **Revert** button restores the
preset values. Persisting changes happens in the preset YAML itself (Presets → Open in
Editor). The live preview shows the exact command line that will run.

The Main view makes the current plan explicit: files group by detected type and each group
header shows the route and the resolved settings it will be processed with (e.g.
`D 0.3 · E 7`, with a `+flags` chip when extra cjxl flags apply); Effort is a simple slider
alongside Distance/Quality. While a conversion runs, an inline progress strip with
pause/cancel and live per-file status keeps the queue visible without leaving the view, and
finished files show their output size and saving ratio in place - the convert bar totals the
whole batch.

*TODO: Screenshot of the expert view with the effort ladder*

After a conversion, click a finished file to run `jxlinfo -v` for its JXL output and show the
detailed metadata. Failed or skipped results without a JXL output report why metadata is
unavailable.

### 2. The command line

```powershell
jxleet.exe "C:\photos\shoot\DSC_0001.jpg" "C:\photos\shoot\DSC_0002.jpg"
jxleet.exe --preset "web-d15-e7" "C:\photos\shoot\"
```

Any number of paths, files or folders, in any mix. With paths given, jxleet uses the preset
bound to the command-line entry point and never shows a settings dialog. `--preset`
overrides that binding for a single call.

`jxleet.exe --help` prints the command-line options and `jxleet.exe --version` prints the
application version. The per-user Explorer entry can be registered or removed with
`--register-context-menu` and `--unregister-context-menu`; registration uses the preset bound
to the context-menu entry point.

An unknown preset name aborts with a non-zero exit code rather than falling back to
something plausible. A typo in an automated pipeline should not quietly convert a hundred
images with the wrong settings.

**Running many instances at once is expected.** Lightroom does not call an external tool
once with a hundred paths; it launches several processes in parallel, each carrying a
handful. jxleet is built for that: the first process to arrive takes ownership, every
subsequent one hands over its paths through a named pipe and exits within milliseconds, and
the arriving batches are coalesced into a single run. You get **one window and one progress
bar**, not twenty — and the calling application is never left waiting.

*TODO: Screenshot of the progress view showing a run coalesced from 20 invocations*

### 3. The Explorer context menu

Right-click any file, folder, or folder background. Registration is per-user and needs no
administrator rights.

The entry carries the name of the preset it will use — **To JXL — archive-lossless** —
because a menu item that can replace your files should say what it does before you click it.

> **On Windows 11** the entry lives under *Show more options*. Reaching the primary context
> menu requires a packaged app with a COM handler, which jxleet does not ship.

## Presets

A preset is a YAML file. It pairs **file filters** with **`cjxl` arguments** — and those
arguments are passed through verbatim. jxleet does not invent a settings vocabulary that
wraps the encoder; the preset *is* the argument list.

On first start jxleet creates three read-only presets — `default-gui`,
`default-cli`, and `default-explorer-context` — and binds each to its corresponding entry
point. Duplicate one to create a writable preset; the built-in defaults cannot be renamed,
deleted, or overwritten.

The selected preset supplies the file rules used for classification. Basic and Expert
controls are temporary run overrides and do not yet mirror every value in a multi-rule
preset; the Presets view shows those format-specific rules.

Presets are edited by changing the YAML files directly. The Preset library has an
**open-folder** button and a **Reload** button so you can edit a file and pull the change back
in without restarting. Every preset jxleet writes carries a `# yaml-language-server` modeline
pointing at a committed `preset.schema.json` (kept next to your presets), so a schema-aware
editor validates and autocompletes your edits. Keep a trailing `"*"` rule as the catch-all;
the built-in defaults and new presets already include one.

```yaml
# %APPDATA%\jxleet\presets\archive-lossless.yaml
name: archive-lossless
description: Keep JPEGs recoverable, everything else mathematically lossless
version: 1

output:
  policy: alongside       # alongside | subfolder | replace
  subfolder: jxl
  on_collision: skip      # skip | number | overwrite

rules:
  # JPEG stays recoverable — the original can be reconstructed from the result
  - match: [JPEG]
    args:
      "--lossless_jpeg": 1

  # Pixel formats: mathematically lossless, high effort, it is an archive
  - match: [PNG, APNG, GIF, PPM, PGM, PAM, PFM, PGX, EXR]
    args:
      "-d": 0
      "-e": 9
      "--num_threads": 8

  # Anything else that cjxl accepts
  - match: ["*"]
    args:
      "-d": 1.0
      "-e": 7
```

**Rules are evaluated top to bottom and the first match wins.** A `"*"` rule at the end acts
as the fallback; without one, unmatched files are skipped and reported.

`match` takes format names as listed under [Supported formats](#supported-formats), or `"*"`.

`args` are keys and values exactly as `cjxl` expects them. Both spellings work, because both
are `cjxl`'s:

```yaml
"-j": 0                     # short form
"--lossless_jpeg": 0        # long form — same flag
```

Flags that take no value are written as `true`:

```yaml
"--progressive": true
```

### A second example

```yaml
name: web-d15-e7
description: Small files for the web, originals replaced
version: 1

output:
  policy: replace
  on_collision: overwrite

rules:
  # Deliberately lossy, including for JPEG — not reversible
  - match: ["*"]
    args:
      "--lossless_jpeg": 0
      "-d": 1.5
      "-e": 7
```

Note what `--lossless_jpeg: 0` does here: it moves every JPEG from the 🟢 transcode route to
the 🟠 reencode route. Combined with `policy: replace`, the originals become unrecoverable.
jxleet will say so, name the number of files, and ask once before starting.

### Validation

Keys are checked against `cjxl -v -v --help` of the **installed** version before a run
starts. An unknown flag stops the run with a non-zero exit code rather than passing it to `cjxl` and hoping for the best.

### Sharing

Presets are plain files. Copy them, commit them, send them. Importing one through the
interface or by dropping it into `%APPDATA%\jxleet\presets\` makes it available immediately.

*TODO: Screenshot of the preset library with the YAML editor*

## The Lightroom workflow (my personal use case)

Export from Lightroom losslessly to JPEG XL, then hand the result to jxleet to apply my
own compression settings. The motivation of this re-encoding step, is that I want develop with lightroom the photos and export them lossless via jxl to re-encode them in a second step with full control over the compression settings.

1. In the Export dialog, set **After Export → Open in Other Application** and select
   `jxleet.exe`.
2. In jxleet, bind the preset you want under **Presets → Command line**.

The parallelism is handled for you, as described above. Remaining time is estimated from
measured throughput over recent files rather than a fixed guess, so it adapts when file
sizes or effort change mid-run.

> **Worth knowing:** re-encoding an already-lossy image quantises it a second time. If your
> Lightroom export is lossless JXL then this is your first lossy step and all is well. If
> you point jxleet at JXL files that were already lossy, it cannot tell.

## Distance, quality and effort

jxleet is an expert tool and does not hide these. All three are `cjxl` concepts; what
follows is a summary, and `cjxl -v -v -v -v --help` remains the authority.

**Distance** (`-d`) is JPEG XL's quality measure in JND units, from `0.0` to `25.0`.
`0` is mathematically lossless. Useful lossy values sit between `0.5` and `3.0`, with
`1.0` visually indistinguishable for most photographic material.

**Quality** (`-q`) on a 0–100 scale is the same quantity in different clothing. In the
interface a toggle switches the display between the two; it never changes the stored value,
so you cannot end up with two settings that disagree. `90` is visually lossless; the
recommended range is `68` to `96`.

The Expert slider colors the recommended bands: Distance `0.5..1.0` green, `1.0..2.0`
dark green, `2.0..3.0` yellow; Quality `86..96` green, `78..86` dark green, and
`68..78` yellow. Values outside those bands remain neutral.

**Effort** (`-e`) trades encoding time for file size, from `1` (lightning) to `10`
(glacier), default `7` (squirrel). The number alone tells you nothing useful, so the expert
view shows an **effort ladder**: a grid of coding tools against the ten levels, lighting up
as you drag. Tools that do not apply to the mode you are in stay visible but struck through,
so you can see what the other mode would buy you.

*TODO: Screenshot of the effort ladder at level 3 and at level 9 side by side*

## Output policies

| Policy | Behaviour |
|---|---|
| `alongside` | Result next to the original. Default, and always the default for imported presets. |
| `subfolder` | Result in `./jxl/` relative to the source, name configurable. |
| `replace` | Result takes the original's place; the original goes to the recycle bin. |

`replace` follows a fixed order that cannot be short-circuited: write to a temporary file in
the target directory, **decode it to prove it is readable** — and on the transcode route
also confirm the reconstructed JPEG is byte-identical — then rename into place, and only
then move the original to the recycle bin. Fail at any step and your original is still
sitting there untouched.

The recycle bin is not optional. jxleet never deletes a file outright, and on volumes
without one (network shares, some removable media) it refuses to replace rather than falling
back to deletion.

### Name collisions

When the target `.jxl` already exists, the preset's `on_collision` decides: `skip` (the safe
default), `number` a new name, `overwrite` silently. Under `skip`, the GUI asks instead of
skipping silently: overwrite this file, overwrite all, skip this file, or skip all — presets
configured for `number` or `overwrite` never prompt.

## The managed toolchain

jxleet does not bundle libjxl. It manages it.

*TODO: Screenshot of the tools panel showing a version mismatch*

Installed versions of `cjxl`, `djxl` and `jxlinfo` are shown on every start and compared
against the latest libjxl release. The binaries are downloaded and saved under `%LOCALAPPDATA%\jxleet\bin\`. 

## Supported formats

**In:** JPEG, PNG, APNG, GIF, OpenEXR, NetPBM (`.pam`, `.pgm`, `.ppm`), PFM, PGX, JXL
**Out:** JXL

These are exactly the formats `cjxl` accepts — jxleet adds none and removes none. Notably
absent:

- **TIFF** — not supported by `cjxl`. Export PNG from your editor instead.
- **RAW** (DNG, CR3, NEF, …) — out of scope. Develop first, then convert.

## Where things are stored

```
%APPDATA%\jxleet\config.yaml       settings and the three entry-point bindings
%APPDATA%\jxleet\presets\          one YAML file per preset
%APPDATA%\jxleet\history.jsonl     one JSON line per successful conversion (History view)
%LOCALAPPDATA%\jxleet\bin\         the managed libjxl binaries
%LOCALAPPDATA%\jxleet\logs\        run logs
```

Uninstalling means deleting `jxleet.exe`, removing the context menu entry from within the
app, and deleting those two folders.

## Building from source

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
git clone https://github.com/dhcgn/jxleet
cd jxleet
wails3 dev      # development build with hot reload
wails3 build    # release build
task check      # build, vet, lint, race tests
```

Requires Go 1.27+ and `cjxl` on `PATH` for the integration tests.
Tests generate their own image fixtures at runtime; there are no binaries in the repository.

## FAQ

**Why not just use `cjxl` directly?**
Do, if that suits you. jxleet adds no encoding capability whatsoever. It adds batching,
preset management, toolchain updates, Lightroom integration and a safety net around
replacing originals.

**Can I get my JPEG back?**
On the transcode route, exactly: `djxl photo.jxl photo.jpg` reproduces the original byte for
byte. On the other routes, no. 

**Why is my JXL bigger than the original?**
Usually effort set too low, or an input that was already aggressively compressed. Try effort
7 or higher and check the size column in the results view.

**Does it strip my metadata?**
No. Exif and XMP are carried across by `cjxl`, and the result view shows what ended up in
the file. But I recommend you check your own workflow.

**Can one preset handle a mixed folder?**
That is what the rules list is for. Put a `[JPEG]` rule first, pixel formats second, `"*"`
last, and one preset covers a folder of anything.

**Does JPEG XL open anywhere yet?**
Support is getting more common, but it is not universal. If you need to share images with people who cannot open JPEG XL yet, consider exporting to a more widely supported format like JPEG.

## License

MIT. See [LICENSE](LICENSE).

Built on [libjxl](https://github.com/libjxl/libjxl) (BSD-3-Clause) and
[Wails](https://wails.io) (MIT). jxleet is an independent front end and is not affiliated
with the libjxl project, the JPEG committee, or Adobe.