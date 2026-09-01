# Effort ladder data

`effort.ts` holds the reference data behind the effort ladder in the Expert
view (`components/EffortLadder.svelte`): which `cjxl` coding tool is active at
which effort level, separately for the lossy (VarDCT) and lossless (Modular)
modes.

## Provenance

The starting point is libjxl's own documentation:
<https://github.com/libjxl/libjxl/blob/main/doc/encode_effort.md>

That document is partially stale (last touched 2025-04-18, before the 2026
streaming/buffering rework). Every row here has been verified against the
libjxl **source** at commit
[`aea3a06e`](https://github.com/libjxl/libjxl/tree/aea3a06e), and where the
document and the source disagree, the source wins:

- **Error diffusion** — the doc lists it at e6; the source gates it at
  effort 7 (`enc_group.cc:418`).
- **Adaptive quantisation at e10** — the doc claims "more thorough"; the
  source runs the identical 4 Butteraugli iterations at e9 and e10
  (`enc_adaptive_quantization.cc:981`), so our row stops at e9.
- **Splines** — not mentioned in the doc at all; on at effort 7
  (`enc_heuristics.cc:1049`).
- **Entropy coding stage boundaries** — the doc only says the search gets
  "slower/more exhaustive as effort goes up"; the boundaries come from
  `HistogramParams` (`enc_ans_params.h:72`, `enc_ans.cc:1345`).

The ladder is reference data for a rendering, **not** generated from the
installed `cjxl` — it does not change with the managed toolchain version.
(The Tools view separately reports when the installed binary differs from
the generated flag snapshot; the ladder is unaffected by that.)

Deliberately not modeled: e11 (expert option, unreachable from the 1–10
slider), the chunked-encoding conditions, and the per-level "more RCTs / more
MA tree properties" increments at e6–e9 (folded into the e6 stage label of
the RCTs row).

## Keeping it current

When a new libjxl release changes `doc/encode_effort.md` **or** the gates
listed below:

1. Diff the upstream document and the cited source locations against this
   file.
2. Update `effortTools` in `effort.ts` and the tables here.
3. Run `npm run check` and `npm run build` in `frontend/`.

## Data model

```ts
interface EffortStage {
  from: number;   // first effort level where this stage applies
  to?: number;    // last effort level (inclusive, default 10)
  label: string;  // cell tooltip text for this stage
}

interface EffortTool {
  name: string;
  tip: string;                 // tooltip on the tool name
  lossy: boolean;              // applies to VarDCT (lossy mode)
  lossless: boolean;           // applies to Modular (lossless mode)
  stages?: EffortStage[];      // default stage list
  stagesLossy?: EffortStage[]; // override for lossy mode
  stagesLossless?: EffortStage[]; // override for lossless mode
  alwaysYellow?: boolean;     // limitation row, always yellow
}
```

- `stages` applies to both modes; `stagesLossy` / `stagesLossless` replace it
  for that mode. Used by *Patches / reference frames* (e5 lossless, e7 lossy)
  and *Entropy coding* (different stage boundaries per mode).
- A stage is active for levels `from` through `to` (default 10). Rows with an
  upper bound — *Fast-lossless path* (e1 only) and *8×8 blocks only* (e1–e4) —
  are lit only at those levels; later levels show nothing.
- Colour: a stage boundary marks a **change in the implementation of the
  feature** — the same tool working in a stronger or more exhaustive form,
  such as adaptive quantisation gaining Butteraugli iterations at e8 or the
  entropy search switching to its most exhaustive mode. It is not an
  on/off distinction. The active stage's rank paints the cell, the strongest
  form always being blue and weaker forms stepping down —
  1 stage = blue · 2 = green, blue · 3 = yellow, green, blue ·
  4 = orange, yellow, green, blue. A single blue row means the tool exists in
  exactly one form and simply switches on.
- `alwaysYellow` marks limitations: *Fast-lossless path* and *8×8 blocks only*
  stay yellow regardless of stage count.
- Cell tooltip: `"<name> (e<from>+): <stage label>"`; inactive cells read
  `"<name>: not active at e<level>"`.

## Transcription tables

Lossless (Modular) column of the upstream table:

| Row | Stages |
|---|---|
| Fast-lossless path | e1: dedicated fast-lossless encoder |
| MA tree | e2: fixed, Gradient-error context · e3: fixed, WP-error context · e4: learned (more properties from e6) · e10: global |
| Predictors | e1: fixed ClampedGradient · e3: fixed Weighted added · e4: both tried · e8: tuned Weighted parameters; all predictors tried at e10 |
| RCTs | e1: fixed YCoCg · e5: different local RCTs · e6: more variants, added through e9 |
| Palette | e1: simple palette detection · e2: global channel palette · e5: local / local channel palette |
| Patches / reference frames | e5: patches and reference frames |
| Entropy coding | e1: Huffman, RLE-only LZ77 · e2: ANS · e8: exhaustive entropy search |

Lossy (VarDCT) column:

| Row | Stages |
|---|---|
| 8×8 blocks only | e1–e4 (limitation; variable blocks take over at e5) |
| Variable block sizes | e5: simple heuristics · e6: full heuristics |
| Coefficient reordering | e4 |
| Adaptive quantisation | e5 · e8: Butteraugli iterations · e9: more iterations (e9 and e10 are identical) |
| Gabor-like transform | e5 |
| Chroma from luma | e5 |
| Error diffusion | e7 |
| Patches / reference frames | e7: patches including dots |
| Splines | e7 |
| Entropy coding | e1: ANS, basic context clustering · e3: better ANS · e9: best context clustering |

## Where to look in the source

All references are permalinks to [`aea3a06e`](https://github.com/libjxl/libjxl/tree/aea3a06e).
The key to reading every gate: effort and `SpeedTier` are mirrored enums
(`speed_tier = 10 − effort` in `jxl_test.cc`), so
`speed_tier <= SpeedTier::kSquirrel` reads *"effort 7 and slower"*:
kLightning e1 · kThunder e2 · kFalcon e3 · kCheetah e4 · kHare e5 ·
kWombat e6 · kSquirrel e7 · kKitten e8 · kTortoise e9 · kGlacier e10.

### Modular (lossless)

- **Fast-lossless path** — eligibility:
  [`encode.cc:2316`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/encode.cc#L2316-L2335)
  (`kLightning` + lossless); fixed YCoCg RCT:
  [`enc_fast_lossless.cc:3015`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_fast_lossless.cc#L3015-L3019);
  ClampedGradient = `Predictor::Gradient`:
  [`context_predict.h:490`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/modular/encoding/context_predict.h#L490);
  palette detection:
  [`enc_fast_lossless.cc:3755`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_fast_lossless.cc#L3755);
  fixed tree / one context:
  [`enc_fast_lossless.cc:2950`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_fast_lossless.cc#L2950-L2958);
  Huffman + RLE-only LZ77:
  [`enc_fast_lossless.cc:2960`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_fast_lossless.cc#L2960-L2984)
- **MA tree** — fixed trees per effort:
  [`enc_modular.cc:1238`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L1238-L1244)
  (e2 `kGradientFixedDC`, e3 `kWPFixedDC`); learned tree:
  [`enc_modular.cc:1173`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L1173)
  (`< kFalcon`, i.e. e4+); tree properties per effort (4/5/7/10/all-16):
  [`enc_modular.cc:561`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L561-L596);
  global MA tree at e10:
  [`enc_frame.cc:1689`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_frame.cc#L1689-L1702)
- **Predictors** — `Predictor::Best` = best of Gradient and Weighted:
  [`options.h:37`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/modular/options.h#L37);
  all predictors (`Predictor::Variable`) at `kGlacier`:
  [`enc_modular.cc:626`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L626-L633);
  WP parameter modes (e8: 2, e9+: 5):
  [`enc_modular.cc:1527`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L1527-L1544)
- **RCTs** — tried per effort (e5:4, e6:5, e7:7, e8:9, e9+:19):
  [`enc_modular.cc:1446`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L1446-L1470)
- **Palette** — global channel palette from e2:
  [`enc_modular.cc:891`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L891-L895);
  local palettes at e5:
  [`enc_modular.cc:1429`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L1429-L1436)
- **Patches** — modular patches `< kCheetah` (e5+):
  [`enc_modular.cc:710`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L710-L718)
- **Entropy coding** — `ForModular` tiers (clustering kFast e1–e7, kBest e8+;
  LZ77 kNone e2–e4 / kRLE e5–e7 / kLZ77b3w3f e8 / kOptc256 e9+):
  [`enc_ans.cc:1345`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_ans.cc#L1345-L1387),
  called from [`enc_modular.cc:687`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_modular.cc#L687)

### VarDCT (lossy)

- **8×8 blocks only / Variable block sizes** — DCT8-only below e5:
  [`enc_ac_strategy.cc:1143`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_ac_strategy.cc#L1143-L1147);
  "simple" (aligned-only, e5) vs "full" (non-aligned, e6+) heuristics:
  [`enc_ac_strategy.cc:1030`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_ac_strategy.cc#L1030-L1057)
- **Entropy coding** — kFastest e1–e2 / kFast e3–e8 / kBest e9–e10:
  [`enc_ans_params.h:72`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_ans_params.h#L72-L87)
- **Coefficient reordering** — off in Falcon or faster (from e4):
  [`enc_coeff_order.cc:40`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_coeff_order.cc#L40-L42)
- **Adaptive quantisation** — initial quant field at `kHare` (e5+):
  [`enc_heuristics.cc:1094`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_heuristics.cc#L1094-L1130);
  Butteraugli iterations at `kKitten` (e8+):
  [`enc_adaptive_quantization.cc:1273`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_adaptive_quantization.cc#L1273-L1289);
  iteration counts (2 default, 4 max, 4 for e9 **and** e10):
  [`enc_adaptive_quantization.cc:926`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_adaptive_quantization.cc#L926-L927)
  and [`:981`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_adaptive_quantization.cc#L981-L984)
- **Gabor-like transform** — [`enc_frame.cc:316`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_frame.cc#L316-L322)
- **Chroma from luma** — [`enc_heuristics.cc:1190`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_heuristics.cc#L1190-L1195)
- **Error diffusion** — `kSquirrel` gate (e7+, not e6 as the doc says):
  [`enc_group.cc:418`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_group.cc#L418-L419)
- **Patches / dots** — `kSquirrel` gate:
  [`enc_heuristics.cc:1059`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_heuristics.cc#L1059-L1066)
  and [`enc_patch_dictionary.cc:632`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_patch_dictionary.cc#L632-L637)
- **Splines** — `FindSplines` at `kSquirrel`:
  [`enc_heuristics.cc:1049`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_heuristics.cc#L1049-L1056)
- **e1 = e2** (kLightning demoted to kThunder internally):
  [`enc_frame.cc:2571`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_frame.cc#L2571-L2575)
- **e11** (not modeled) — range check:
  [`encode.cc:1668`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/encode.cc#L1668-L1682);
  lossless-only demotion to e10:
  [`enc_frame.cc:2567`](https://github.com/libjxl/libjxl/blob/aea3a06e/lib/jxl/enc_frame.cc#L2567-L2570)
