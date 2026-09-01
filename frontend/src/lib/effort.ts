export const effortNames = [
  'lightning',
  'thunder',
  'falcon',
  'cheetah',
  'hare',
  'wombat',
  'squirrel',
  'kitten',
  'tortoise',
  'glacier',
];

export interface EffortStage {
  from: number;
  to?: number;
  label: string;
}
export interface EffortTool {
  name: string;
  tip: string;
  lossy: boolean;
  lossless: boolean;
  stages?: EffortStage[];
  stagesLossy?: EffortStage[];
  stagesLossless?: EffortStage[];
  alwaysYellow?: boolean;
}

// Reference data for the Expert-view effort ladder. Documented, including
// provenance and update procedure, in effort.md next to this file.
// Transcribed from libjxl doc/encode_effort.md; splines verified in the source.
export const effortTools: EffortTool[] = [
  {
    name: 'Fast-lossless path',
    tip: 'e1 runs a dedicated fast-lossless encoder; the general modular pipeline starts at e2',
    lossy: false,
    lossless: true,
    stages: [{ from: 1, to: 1, label: 'dedicated fast-lossless encoder' }],
    alwaysYellow: true,
  },
  {
    name: 'MA tree',
    tip: 'meta-adaptive tree, the context model of the modular entropy coder',
    lossy: false,
    lossless: true,
    stages: [
      { from: 2, label: 'fixed tree, context from Gradient error' },
      { from: 3, label: 'fixed tree, context from WP error' },
      { from: 4, label: 'learned tree (more properties from e6)' },
      { from: 10, label: 'global tree' },
    ],
  },
  {
    name: 'Predictors',
    tip: 'modular-mode pixel predictors',
    lossy: false,
    lossless: true,
    stages: [
      { from: 1, label: 'fixed ClampedGradient predictor' },
      { from: 3, label: 'fixed Weighted predictor added' },
      { from: 4, label: 'ClampedGradient and Weighted both tried' },
      { from: 8, label: 'tuned Weighted parameters; all predictors tried at e10' },
    ],
  },
  {
    name: 'RCTs',
    tip: 'reversible colour transforms applied before prediction',
    lossy: false,
    lossless: true,
    stages: [
      { from: 1, label: 'fixed YCoCg' },
      { from: 5, label: 'different local RCTs' },
      { from: 6, label: 'more RCT variants, added through e9' },
    ],
  },
  {
    name: 'Palette',
    tip: 'indexed-colour content encoded as a palette',
    lossy: false,
    lossless: true,
    stages: [
      { from: 1, label: 'simple palette detection' },
      { from: 2, label: 'global channel palette' },
      { from: 5, label: 'local palette / local channel palette' },
    ],
  },
  {
    name: 'Patches / reference frames',
    tip: 'repeating content stored once and referenced elsewhere',
    lossy: true,
    lossless: true,
    stages: [{ from: 5, label: 'patches and reference frames' }],
    stagesLossy: [{ from: 7, label: 'patches including dots' }],
  },
  {
    name: 'Entropy coding',
    tip: 'Huffman vs ANS, and how exhaustive the entropy search is',
    lossy: true,
    lossless: true,
    stagesLossless: [
      { from: 1, label: 'Huffman, RLE-only LZ77' },
      { from: 2, label: 'ANS' },
      { from: 8, label: 'exhaustive entropy search' },
    ],
    stagesLossy: [
      { from: 1, label: 'ANS, basic context clustering' },
      { from: 3, label: 'better ANS' },
      { from: 9, label: 'best context clustering' },
    ],
  },
  {
    name: '8×8 blocks only',
    tip: 'VarDCT limited to 8×8 DCT blocks; variable block sizes start at e5',
    lossy: true,
    lossless: false,
    stages: [{ from: 1, to: 4, label: 'only 8×8 DCT blocks' }],
    alwaysYellow: true,
  },
  {
    name: 'Variable block sizes',
    tip: 'VarDCT blocks larger than 8×8, chosen by heuristics',
    lossy: true,
    lossless: false,
    stages: [
      { from: 5, label: 'simple heuristics' },
      { from: 6, label: 'full heuristics' },
    ],
  },
  {
    name: 'Coefficient reordering',
    tip: 'reordering of DCT coefficients before entropy coding',
    lossy: true,
    lossless: false,
    stages: [{ from: 4, label: 'coefficient reordering' }],
  },
  {
    name: 'Adaptive quantisation',
    tip: 'per-block quantisation driven by the psychovisual model',
    lossy: true,
    lossless: false,
    stages: [
      { from: 5, label: 'adaptive quantisation' },
      { from: 8, label: 'Butteraugli iterations' },
      { from: 9, label: 'more Butteraugli iterations (e9 and e10 are identical)' },
    ],
  },
  {
    name: 'Gabor-like transform',
    tip: 'blur-like filtering of the XYB channels before the DCT',
    lossy: true,
    lossless: false,
    stages: [{ from: 5, label: 'gabor-like transform' }],
  },
  {
    name: 'Chroma from luma',
    tip: 'correlates chroma DCT coefficients with the luma channel',
    lossy: true,
    lossless: false,
    stages: [{ from: 5, label: 'chroma from luma' }],
  },
  {
    name: 'Error diffusion',
    tip: 'spreads quantisation error across neighbouring pixels',
    lossy: true,
    lossless: false,
    stages: [{ from: 7, label: 'error diffusion' }],
  },
  {
    name: 'Splines',
    tip: 'thin curve-like features (verified in the libjxl source; not in encode_effort.md)',
    lossy: true,
    lossless: false,
    stages: [{ from: 7, label: 'spline detection' }],
  },
];

export function stageIndexAt(stages: EffortStage[], level: number): number {
  let index = -1;
  for (let i = 0; i < stages.length; i++) {
    if (stages[i].from <= level && (stages[i].to ?? 10) >= level) index = i;
  }
  return index;
}
