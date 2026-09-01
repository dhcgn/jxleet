import type { RouteMode } from './types';

// Port of libjxl JxlEncoderDistanceFromQuality (lib/jxl/encode.cc) — the
// authority for how -q maps to -d.
export function distanceFromQuality(quality: number): number {
  return quality >= 100 ? 0 : quality >= 30 ? 0.1 + (100 - quality) * 0.09 : (53 / 3000) * quality * quality - (23 / 20) * quality + 25;
}

export function qualityFromDistance(value: number): number {
  if (value <= 0) return 100;
  if (value <= 6.4) return Math.max(0, Math.min(100, 100 - (value - 0.1) / 0.09));
  const a = 53 / 3000;
  const b = -23 / 20;
  const discriminant = b * b - 4 * a * (25 - value);
  return Math.max(0, Math.min(100, (-b - Math.sqrt(Math.max(0, discriminant))) / (2 * a)));
}

// Shared slider-track bands, in distance units: purple below 0.5, dark green
// up to 1.0, green to 1.5, yellow to 2.0, orange to 3.0, red beyond.
// The sliders stop at distance 5 (quality 46); heavier compression means cjxl directly.
export const SLIDER_BANDS = [0.5, 1, 1.5, 2, 3] as const;
export const SLIDER_DISTANCE_MAX = 5;
export const SLIDER_QUALITY_MIN = Math.ceil(qualityFromDistance(SLIDER_DISTANCE_MAX));

// step 0.025, always displayed with three decimals: 0.000
export const formatDistance = (d: number): string => d.toFixed(3);

export function qualityStatusText(routeMode: RouteMode, distance: number, outOfRange: boolean): string {
  if (routeMode === 'lossless') return 'Lossless: distance fixed at 0';
  if (distance <= 1.0) return 'visually lossless';
  if (distance >= distanceFromQuality(SLIDER_QUALITY_MIN)) return 'slider limit — for more range use the cjxl command line';
  return outOfRange ? 'Out of recommended range (too high or too low)' : 'Inside recommended range';
}

const zoneStyle = (toSliderPercent: (band: number) => number): string =>
  SLIDER_BANDS.map((band, index) => `--zone-${'abcde'[index]}:${toSliderPercent(band).toFixed(2)}%`).join(';');

export const distanceZoneStyle = zoneStyle((band) => (band / SLIDER_DISTANCE_MAX) * 100);
// The quality slider is inverted (position = 100 - quality) and spans 46..100.
export const qualityZoneStyle = zoneStyle((band) => ((100 - qualityFromDistance(band)) / (100 - SLIDER_QUALITY_MIN)) * 100);
