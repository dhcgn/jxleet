import type { FilePreview } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';

export type View = 'main' | 'expert' | 'presets' | 'tools' | 'automatic' | 'history';
export type RouteMode = 'lossy' | 'lossless';

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
