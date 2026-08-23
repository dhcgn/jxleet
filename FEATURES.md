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
- [ ] Use `go:generate` to keep the available flags in sync with cjxl, trough call of `cjxl --help -v -v -v -v` and parsing the output

## Presets
- [ ] Create, duplicate, rename, delete named presets 
- [ ] Separate binding per entry point 
- [ ] Export to a file 
- [ ] Import from a file, without adopting the output policy 
- [ ] Format version with a migration path 
- [ ] Collision handling on import 

## Output
- [ ] Alongside 
- [ ] Into a subfolder 
- [ ] Replace via recycle bin, after verification 
- [ ] Deletion rule selectable per route 
- [ ] Separate confirmation for irreversible routes 
- [ ] Handle name collisions 

## Entry points
- [ ] Graphical interface 
- [ ] File-path invocation without prompting 
- [ ] Context menu for files, folders, folder background 
- [ ] Folders recursively 
- [ ] Preset name overridable by flag 
- [ ] Preset name visible in the menu text 

## Concurrency
- [ ] Single instance 
- [ ] Handover to the running instance, second process returns immediately 
- [ ] Coalesce invocations into one run 
- [ ] Takeover when the instance is unreachable 
- [ ] Pause and cancel 
- [ ] Processes and threads configurable separately 
- [ ] Progress with remaining time from measured throughput 

## Toolchain management
- [ ] Show versions of cjxl, djxl, jxlinfo 
- [ ] Compare with the latest release 
- [ ] Download with checksum verification 
- [ ] Atomic update 
- [ ] Lock expert flags on version mismatch 
- [ ] Offer first-time installation of the toolchain 
- [ ] Diff the flag list on version bump 

## Interface

> Take a look at [develop-time\design\jxlconv-mockups.html](develop-time\design\jxlconv-mockups.html) for the mockups that guided the design of the GUI.

- [ ] Dark by default, operable from 420 pixels 
- [ ] Basic and expert separated 
- [ ] Drop and file dialog 
- [ ] Colour marking of routes, follows the setting 
- [ ] Distance/quality toggle 
- [ ] Effort slider with effort ladder 
- [ ] Display of the command line to be executed 
- [ ] Progress, remaining time, throughput, failures 
- [ ] Note about coalesced invocations 
- [ ] Size balance per file and in total 
- [ ] jxlinfo key figures including reconstruction data 
- [ ] Compact window for the automatic invocation 
- [ ] Log view with tool messages 
