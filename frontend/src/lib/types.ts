import type { ConversionOptions, FilePreview } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';

export type View = 'main' | 'expert' | 'queue' | 'presets' | 'tools' | 'automatic' | 'history' | 'stats';
export type RouteMode = 'lossy' | 'lossless';

export type QueueStatus = 'waiting' | 'processing' | 'done' | 'failed' | 'cancelled' | 'skipped';

// QueueItem is one staged file with a frozen settings snapshot: later preset
// edits never touch it, and the same file may be queued twice with different
// settings (ref:jl:domain.queue.item).
export interface QueueItem {
  id: string;
  path: string;
  name: string;
  size: number;
  format: string;
  route: string;
  addedAt: number;
  preset: string;
  snapshot: string; // frozen label: distance with quality, effort, +flags hint
  flagsSet: boolean;
  options: ConversionOptions; // frozen per-item run options
  status: QueueStatus;
  pid: number;
  startedAt: number;
  output: string;
  outputSize: number;
  error: string;
  warning: string;
  skipped: boolean;
  skipReason: string;
  cancelled: boolean;
  durationSeconds: number;
  cpuPercent: number | null;
  memoryBytes: number | null;
}

export interface FileGroup {
  key: string;
  format: string;
  route: string;
  skip: boolean;
  reason: string;
  settings: string;
  flagsSet: boolean;
  files: FilePreview[];
  sizeIn: number;
  sizeOut: number;
  sizeDoneIn: number;
  hasResults: boolean;
}
