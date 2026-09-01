<script lang="ts">
  import { distanceZoneStyle, formatDistance, qualityStatusText, qualityZoneStyle, SLIDER_QUALITY_MIN } from '../lib/quality';
  import type { RouteMode } from '../lib/types';

  interface Props {
    distance: number;
    quality: number;
    outOfRange: boolean;
    routeMode: RouteMode;
    locked?: boolean;
    onDistance: (value: number) => void;
    onQuality: (quality: number) => void;
  }
  let { distance, quality, outOfRange, routeMode, locked = false, onDistance, onQuality }: Props = $props();
</script>

<div style="display:flex;align-items:baseline;gap:8px;margin-bottom:2px">
  <span class="k">Distance</span>
  <span class="qnum" class:out-of-range={outOfRange}>{formatDistance(distance)}</span>
  <span class="mini range-status" class:out-of-range={outOfRange} style="margin-left:auto">{qualityStatusText(routeMode, distance, outOfRange)}</span>
</div>
<div class="quality-range" class:locked style={distanceZoneStyle}>
  <input type="range" min="0" max="5" step="0.025" value={distance} oninput={(event) => onDistance(Number((event.currentTarget as HTMLInputElement).value))} disabled={locked} aria-label="Distance" />
</div>
<div class="quality-guidance"><span>Recommended: 0.5 .. 3.0</span><span>1.0 = visually lossless</span></div>
<div style="display:flex;align-items:baseline;gap:8px;margin-top:10px;margin-bottom:2px">
  <span class="k">Quality</span>
  <span class="qnum">{quality}</span>
</div>
<div class="quality-range" class:locked style={qualityZoneStyle}>
  <input type="range" min="0" max={100 - SLIDER_QUALITY_MIN} step="1" value={100 - quality} oninput={(event) => onQuality(100 - Number((event.currentTarget as HTMLInputElement).value))} disabled={locked} aria-label="Quality" />
</div>
<div class="quality-guidance"><span>Recommended: 68 .. 96</span><span>90 = visually lossless</span></div>
