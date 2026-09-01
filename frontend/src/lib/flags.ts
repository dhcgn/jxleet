import type { FlagInfo, FlagOverride } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';

export const hiddenExpertFlags = new Set(['--verbose', '--help', '--version', '--quiet']);

// Flags the GUI controls own; they stay read-only in the Expert flag list.
export function isLinkedFlagKey(key: string): boolean {
  return ['--distance', '--quality', '--effort', '--lossless_jpeg', '--num_threads'].includes(key);
}

export function flagLabel(flag: FlagInfo): string {
  const short = flag.short ? `-${flag.short}` : '';
  const long = flag.long ? `--${flag.long}` : '';
  return short && long ? `${short}, ${long}` : short || long;
}

export function sameFlags(a: FlagOverride[], b: FlagOverride[]): boolean {
  if (a.length !== b.length) return false;
  const key = (flag: FlagOverride) => `${flag.key}${flag.value}${flag.valueless}`;
  const left = a.map(key).sort();
  const right = b.map(key).sort();
  return left.every((value, index) => value === right[index]);
}
