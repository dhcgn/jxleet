export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = -1;
  do {
    size /= 1024;
    unit += 1;
  } while (size >= 1024 && unit < units.length - 1);
  return `${size.toFixed(size >= 100 ? 0 : 1)} ${units[unit]}`;
}

export function formatEta(seconds: number): string {
  if (!seconds || seconds < 1) return '--:--';
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.floor(seconds % 60);
  return `${minutes}:${remainder.toString().padStart(2, '0')}`;
}

export function formatRate(value: number): string {
  return value > 0 ? `${formatBytes(value)}/s` : '--';
}

export function savedPct(before: number, after: number): number {
  if (before <= 0) return 0;
  return Math.round((1 - after / before) * 100);
}

export function formatDelta(before: number, after: number): string {
  const pct = savedPct(before, after);
  return pct >= 0 ? `-${pct}%` : `+${-pct}%`;
}

export function compactPath(path: string, maxLength = 80): string {
  if (path.length <= maxLength) return path;
  const tail = Math.max(16, Math.floor(maxLength * 0.35));
  const head = Math.max(3, maxLength - tail - 3);
  return `${path.slice(0, head)}...${path.slice(-tail)}`;
}

// Needed-time display for Queue/History rows: m:ss past a minute, seconds
// below. Callers render a placeholder when the value is unknown (0/null).
export function formatDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return '—';
  if (seconds < 60) return seconds < 10 ? `${seconds.toFixed(1)}s` : `${Math.round(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.floor(seconds % 60);
  return `${minutes}:${remainder.toString().padStart(2, '0')}`;
}

// History timestamps are "YYYY-MM-DD HH:mm:ss" local time.
export function formatHistoryAt(when: Date): string {
  const pad = (value: number): string => value.toString().padStart(2, '0');
  return `${when.getFullYear()}-${pad(when.getMonth() + 1)}-${pad(when.getDate())} ${pad(when.getHours())}:${pad(when.getMinutes())}:${pad(when.getSeconds())}`;
}
