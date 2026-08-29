---
description: Cut a GitHub release by pushing a vX.Y.Z tag; CI builds and publishes.
---

Create a GitHub release for jxleet by pushing a version tag. CI does everything else.

How releasing works here (`.github/workflows/release.yml`):

- Push a tag `vX.Y.Z` → the Release workflow (windows-latest) runs `task build`, zips
  `jxleet.exe` + `SHA256SUMS`, and publishes a GitHub Release with generated notes.
- A tag containing a hyphen (`v1.3.0-rc.1`) publishes as a **pre-release** automatically.
- Betas (`vX.Y.Z-beta.N`) come from pushes to `dev` — not part of this command.
- Nothing is built locally; the tag push is the whole release action.

Steps:

1. **Version**: use `$ARGUMENTS` if given (accepts `vX.Y.Z`, `X.Y.Z`, or pre-release
   forms; normalize to a leading `v`). Otherwise list existing tags
   (`git tag -l 'v*' --sort=-v:refname`), propose the next version, and ask me
   which bump (major/minor/patch) before proceeding.
2. **Preflight**: stop and report instead of fixing if any of these fail:
   - `git status --porcelain` is not clean (uncommitted work must not ship),
   - current branch is not `main`, or `git fetch --tags origin` shows main is
     behind `origin/main`,
   - `task check` fails.
3. **Release**: `git tag -a <tag> -m "<tag>"` then `git push origin <tag>`.
4. **Watch the workflow run** (the push is only done once the run is verified green):
   - The run appears a few seconds after the push (sleep ~10s first, or the list
     comes back empty). Find it:
     `gh run list -w Release --limit 3` — the newest run, status `in_progress`/`queued`.
   - Watch it to completion (this takes ~4 minutes):
     `gh run watch <id> --exit-status --interval 15`
     `--exit-status` makes the command exit non-zero on failure, so a failure
     can't slip through as success.
   - While watching, look for the `Release (windows)` job: all steps ✓ —
     `Build`, `Zip and checksums`, `Create release` are the ones that matter.
5. **Verify the release exists**: `gh release view <tag>` (or with
   `--json name,tagName,isPrerelease,url,assets` for machine-readable output) —
   check `isPrerelease` matches the tag form (no hyphen = `false`) and both
   assets are there: `jxleet_<version>_windows_amd64.zip` + `SHA256SUMS`.
   Report the release URL to me.
6. **If the run fails**: fetch the log of the failed step with
   `gh run view <id> --log-failed` and show it to me. Do not delete or re-push
   the tag on your own — an already-published release must never be moved.
