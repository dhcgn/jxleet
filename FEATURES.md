# Feature list

Complete inventory as a checklist. The bracket names the spec area.

## Conversion
- [ ] Transcode JPEG, bit-exactly reversible 
- [ ] Reencode JPEG lossily 
- [ ] Reencode JXL (from or to lossless or lossy)
- [ ] Encode other formats that are compatible with cjxl to jxl lossless or lossy 
- [ ] Determine route (what to do) from file type and usage (GUI, CLI, context menu)
- [ ] Distance and quality as one quantity with two displays (GUI can switch between them)
- [ ] Effort 1–10 
- [ ] Skip unsupported files instead of aborting 
- [x] GUI Expert Mode with all flags of cjxl
- [x] Use `go:generate` to keep the available flags in sync with cjxl, trough call of `cjxl --help -v -v -v -v` and parsing the output

## Presets
- [x] Create, duplicate, rename, delete named presets 
- [x] Separate binding per entry point
- [x] Export to a file 
- [x] Import from a file, without adopting the output policy 
- [x] Format version with a migration path 
- [x] Collision handling on import 
- [x] Read-only entry-point defaults bound on first start
- [x] Guaranteed catch-all (`*`) rule in defaults and new presets
- [x] JSON schema file for editor validation, referenced from every preset
- [x] Reload preset library from the folder on demand
- [ ] Synchronize every multi-rule preset value into global GUI controls (deferred design)

## Output
- [x] Alongside 
- [x] Into a subfolder 
- [x] Replace via recycle bin, after verification 
- [x] Deletion rule selectable per route 
- [x] Separate confirmation for irreversible routes
- [x] Handle name collisions 

## Entry points
- [x] Graphical interface
- [x] File-path invocation without prompting
- [x] Context menu for files, folders, folder background
- [x] Direct folder files only (no recursion)
- [x] Preset name overridable by flag
- [x] Preset name visible in the menu text

## Concurrency
- [x] Single instance 
- [x] Handover to the running instance, second process returns immediately 
- [x] Coalesce invocations into one run
- [x] Auto-start a new run when a handover arrives after the previous run finished
- [x] Takeover when the instance is unreachable 
- [x] Pause and cancel 
- [x] Processes and threads configurable separately 
- [x] Progress with remaining time from measured throughput 

## Toolchain management
- [x] Show versions of cjxl, djxl, jxlinfo
- [x] Compare with the latest release
- [x] Download with checksum verification
- [x] Download progress reported to the UI (phase + bytes)
- [x] Atomic update
- [x] Lock expert flags on version mismatch
- [x] Offer first-time installation of the toolchain
- [x] Diff the flag list on version bump

## Interface

> Take a look at [develop-time\design\jxlconv-mockups.html](develop-time\design\jxlconv-mockups.html) for the mockups that guided the design of the GUI.

- [x] Dark by default, operable from 420 pixels
- [x] Main and expert views (drop, running and done are inline states of Main)
- [x] Drop anywhere in the window; file dialog as alternative
- [x] Colour marking of routes, follows the setting
- [x] Distance/quality toggle
- [x] Effort slider with effort ladder
- [x] Effort as a simple slider in the Main view
- [x] Files grouped by detected type with resolved settings (distance/quality, effort, flags) per group
- [x] Preset strip below the toolbar spelling out every rule of the active preset
- [x] Presets seed the controls; edits are session-only overrides with amber warning + Revert; persistence via YAML (Open in Editor)
- [x] Queue and live per-file status inline during processing; results (size balance) in place
- [x] Persistent Open File/Open Folder intake and native drops across views
- [x] Expert flag edits as session overrides with cjxl help tooltips and reset to preset values
- [x] Explicit libjxl update check in Tools
- [x] Display of the command line to be executed
- [x] Progress, remaining time, throughput, failures
- [x] Note about coalesced invocations
- [x] Size balance per file and in total
- [x] Live savings counted only from converted files (not zero-filled pending)
- [x] Selectable results with detailed jxlinfo metadata
- [x] Compact window for the automatic invocation
- [x] Clear All button to remove the entire queue
- [x] Session-wide counters: images converted and space saved in the statusbar
- [x] Group header settings frozen during a running conversion
- [x] Adding files after a finished run converts only the new files
- [ ] Log view with tool messages 
