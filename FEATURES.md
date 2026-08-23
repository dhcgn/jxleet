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
- [ ] GUI Expert Mode with all flags of cjxl
- [x] Use `go:generate` to keep the available flags in sync with cjxl, trough call of `cjxl --help -v -v -v -v` and parsing the output

## Presets
- [x] Create, duplicate, rename, delete named presets 
- [ ] Separate binding per entry point 
- [x] Export to a file 
- [x] Import from a file, without adopting the output policy 
- [x] Format version with a migration path 
- [x] Collision handling on import 

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
- [x] Folders recursively
- [x] Preset name overridable by flag
- [x] Preset name visible in the menu text

## Concurrency
- [x] Single instance 
- [x] Handover to the running instance, second process returns immediately 
- [x] Coalesce invocations into one run
- [x] Takeover when the instance is unreachable 
- [x] Pause and cancel 
- [x] Processes and threads configurable separately 
- [x] Progress with remaining time from measured throughput 

## Toolchain management
- [x] Show versions of cjxl, djxl, jxlinfo
- [x] Compare with the latest release
- [x] Download with checksum verification
- [x] Atomic update
- [x] Lock expert flags on version mismatch
- [x] Offer first-time installation of the toolchain
- [x] Diff the flag list on version bump

## Interface

> Take a look at [develop-time\design\jxlconv-mockups.html](develop-time\design\jxlconv-mockups.html) for the mockups that guided the design of the GUI.

- [x] Dark by default, operable from 420 pixels
- [x] Basic and expert separated
- [x] Drop and file dialog
- [x] Colour marking of routes, follows the setting
- [x] Distance/quality toggle
- [x] Effort slider with effort ladder
- [x] Display of the command line to be executed
- [x] Progress, remaining time, throughput, failures
- [x] Note about coalesced invocations
- [x] Size balance per file and in total
- [ ] jxlinfo key figures including reconstruction data
- [x] Compact window for the automatic invocation
- [ ] Log view with tool messages 
