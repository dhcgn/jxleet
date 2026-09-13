---
description: Check jl: glossary usage — unknown keys, orphaned definitions, missing terms. Read-only.
---

Check the project glossary (`docs/GLOSSARY.md`, defined by `jl:` source comments, chapters in `docs/GLOSSARY.topics.yaml`) against actual usage. **Read-only: never edit sources, only report.**

1. **Collect defined keys**: read the `` `jl:...` `` entries in `docs/GLOSSARY.md` (and the facets in `docs/GLOSSARY.topics.yaml`).
2. **Find usages**: search the repo for `jl:[a-z0-9.~-]+` in code comments, docs, and (for the current change) issue/PR text. In `.go`/`.ts` only `//` comments count; in `.svelte` `//`, `<!-- -->` and `/* */`; in `.yaml`/`.yml`/`.ps1` `#`; in `.md` `<!-- -->` defines keys while full prose may carry `ref:` mentions (ignore `docs/GLOSSARY.md` itself and `*_test.go` / `*.test.ts` / `*.spec.ts` fixtures). The `glossary-search` agent skill (`defs` / `refs` / `keys` modes) automates these searches.
3. **Report four lists**:
   - **Unknown keys**: used but not defined in `docs/GLOSSARY.md`.
   - **Orphaned definitions**: defined but never referenced outside their defining comment and the generated file.
   - **Dangling references**: `ref:` mentions with no matching definition — these fail `go generate ./...` (Broken references section), so they outrank everything else.
   - **Missing-term candidates**: user-visible names (routes, policies, views, bindings, toolchain behaviour) used in the change that look like they deserve an entry but have none.
4. End with the single most important gap, if any. Do not fix anything — fixing means adding a `jl:` comment at the anchor file and running `go generate ./...`, which is a separate change.
