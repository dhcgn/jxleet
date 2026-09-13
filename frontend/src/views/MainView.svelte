<!-- jl:view.main=Main view: flat file intake in added order with route badges, session overrides and the move-to-queue bar. -->
<script lang="ts">
  import type { FilePreview, Status, ToolchainProgress, ToolchainStatus } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { formatBytes } from '../lib/format';
  import { effortNames } from '../lib/effort';
  import { routeClass, routeTitle } from '../lib/routes';
  import type { RouteMode } from '../lib/types';
  import QualitySliders from '../components/QualitySliders.svelte';

  interface Props {
    files: FilePreview[];
    settings: { distance: number; effort: number; jpegLossless: boolean; outputPolicy: string; embedSettings: boolean; jxlInfoSidecar: boolean };
    routeMode: RouteMode;
    quality: number;
    outOfRange: boolean;
    presetName: string;
    appStatus: Status | null;
    tools: { status: ToolchainStatus | null; installing: boolean; progress: ToolchainProgress | null };
    canQueue: boolean;
    onOpenFile(): void;
    onOpenFolder(): void;
    onInstallToolchain(): void;
    onGoToPresets(): void;
    onClearAll(): void;
    onSetDistance(value: number): void;
    onSetQuality(quality: number): void;
    onSetEffort(value: number): void;
    onSetJpegMode(lossless: boolean): void;
    onSetOutputPolicy(policy: 'alongside' | 'subfolder' | 'replace'): void;
    onSetEmbedSettings(embed: boolean): void;
    onSetJxlInfoSidecar(sidecar: boolean): void;
    onMoveToQueue(): void;
  }
  let {
    files,
    settings,
    routeMode,
    quality,
    outOfRange,
    presetName,
    appStatus,
    tools,
    canQueue,
    onOpenFile,
    onOpenFolder,
    onInstallToolchain,
    onGoToPresets,
    onClearAll,
    onSetDistance,
    onSetQuality,
    onSetEffort,
    onSetJpegMode,
    onSetOutputPolicy,
    onSetEmbedSettings,
    onSetJxlInfoSidecar,
    onMoveToQueue,
  }: Props = $props();

  let routeCounts = $derived.by(() => ({
    Transcode: files.filter((file) => file.route === 'Transcode').length,
    Reencode: files.filter((file) => file.route === 'Reencode').length,
    Encode: files.filter((file) => file.route === 'Encode').length,
    Skip: files.filter((file) => file.skip).length,
  }));
  let totalSize = $derived(files.reduce((total, file) => total + file.size, 0));
</script>

<div class="body">
  {#if tools.installing}
    <div class="run-strip" data-testid="install-strip">
      <div class="run-head">
        <span class="badge b-encode">Installing</span>
        <span class="run-count">{tools.progress?.phase === 'downloading' ? 'Downloading libjxl' : 'Verifying & installing'}</span>
        {#if tools.progress?.phase === 'downloading'}
          <span class="mini">{formatBytes(tools.progress.downloaded)} of {tools.progress.total > 0 ? formatBytes(tools.progress.total) : '?'}</span>
        {/if}
      </div>
      {#if tools.progress?.phase === 'downloading' && tools.progress.total > 0}
        <div class="bar"><i style={`width:${Math.min(100, (tools.progress.downloaded / tools.progress.total) * 100)}%`}></i></div>
      {/if}
    </div>
  {:else if tools.status?.needsInstall}
    <div class="banner warn">
      <span class="ic">!</span>
      <span><b>libjxl is not installed.</b> Install the managed cjxl/djxl/jxlinfo toolchain before converting.</span>
      <button class="btn primary" style="margin-left:auto;background:var(--p-encode)" onclick={onInstallToolchain} disabled={tools.installing}>Install</button>
    </div>
  {/if}
  {#if appStatus && !appStatus.ready}
    <div class="banner info">
      <span class="ic">i</span>
      <span>Bind a preset for each entry point before automated runs.</span>
      <button class="btn ghost" style="margin-left:auto" onclick={onGoToPresets}>Set bindings</button>
    </div>
  {/if}
  {#if presetName === '' && files.length > 0}
    <div class="banner info" style="margin-bottom:12px"><span class="ic">i</span><span>Files are selected. Select a preset in the toolbar to classify their routes.</span></div>
  {/if}
  <div class="cols">
    <div class="groups-col" style="--custom-contextmenu: file-table; --default-contextmenu: hide">
      {#if files.length === 0}
      <div
        class="drop"
        aria-label="Drop files or folders"
      >
      <div class="big">Drop files or folders here</div>
      <div class="sub">jxleet detects the input format and chooses the route. Unsupported files are skipped and reported.</div>
      <div style="display:flex;gap:14px;justify-content:center;margin-top:6px;flex-wrap:wrap">
        <span class="badge b-transcode">JPEG - transcode</span>
        <span class="badge b-reencode">JPEG / JXL - reencode</span>
        <span class="badge b-encode">Pixel - encode</span>
      </div>
      <div class="sub" style="margin-top:10px">or use one of the native open actions</div>
      <div style="display:flex;gap:8px;justify-content:center;margin:4px auto 0;flex-wrap:wrap">
        <button class="btn primary" style="background:var(--p-encode)" onclick={onOpenFile}>Open File</button>
        <button class="btn" onclick={onOpenFolder}>Open Folder</button>
      </div>
    </div>
      {:else}
      <div class="card group" data-testid="group-intake">
        <div class="group-head">
          <span class="badge b-encode">intake</span>
          <span class="group-title">Files in added order</span>
          <span class="mini">{files.length} files - {formatBytes(totalSize)}</span>
          <span class="spacer"></span>
          {#if routeCounts.Transcode > 0}<span class="mini" title={routeTitle('Transcode')}>🟢 {routeCounts.Transcode}</span>{/if}
          {#if routeCounts.Reencode > 0}<span class="mini" title={routeTitle('Reencode')}>🟠 {routeCounts.Reencode}</span>{/if}
          {#if routeCounts.Encode > 0}<span class="mini" title={routeTitle('Encode')}>🔵 {routeCounts.Encode}</span>{/if}
          {#if routeCounts.Skip > 0}<span class="mini">skip {routeCounts.Skip}</span>{/if}
        </div>
        <table class="files group-files" data-testid="file-table">
          <colgroup>
            <col class="gf-file" />
            <col class="gf-hug" />
            <col class="gf-hug" />
            <col class="gf-status" />
          </colgroup>
          <thead><tr><th>File</th><th style="text-align:right">Size</th><th>Route</th><th>Settings</th></tr></thead>
          <tbody>
            {#each files as file (file.path)}
              <tr>
                <td class="fn" title={file.path}>{file.name}</td>
                <td class="num">{formatBytes(file.size)}</td>
                <td>
                  {#if file.skip}
                    <span class="badge b-skip">skip</span>
                  {:else}
                    <span class={`badge ${routeClass(file.route)}`}>{file.format || 'file'}</span>
                  {/if}
                </td>
                <td class="status-cell">
                  {#if file.skip}
                    {file.reason || 'skipped'}
                  {:else}
                    <span class="mono-mini" title={file.flagsSet ? `${file.settings} + extra flags` : file.settings}>{file.settings || '-'}</span>
                    {#if file.flagsSet}<span class="flag-chip" title="Extra cjxl flags applied">+flags</span>{/if}
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="mini" style="padding:0 2px">Drop more files or folders anywhere in the window to add them. Conversion runs from the Queue — staged files keep a frozen copy of these settings.</div>
      {/if}
    </div>

    <div style="display:flex;flex-direction:column;gap:12px">
      <div class="card">
        <h3>Compression <span class="r">stored in the preset</span></h3>
        <div class="in">
          <QualitySliders distance={settings.distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} onDistance={onSetDistance} onQuality={onSetQuality} />
          <div class="effort-basic" style="border-top:1px solid var(--line-soft);margin-top:8px;padding-top:8px">
            <div style="display:flex;align-items:baseline;gap:8px">
              <span class="k">Effort</span>
              <span class="v">{settings.effort} - {effortNames[settings.effort - 1]}</span>
              <span class="mini" style="margin-left:auto">{settings.effort === 7 ? 'default' : settings.effort >= 9 ? 'slow' : settings.effort <= 3 ? 'fast' : ''}</span>
            </div>
            <input type="range" min="1" max="10" step="1" value={settings.effort} oninput={(event) => onSetEffort(Number((event.currentTarget as HTMLInputElement).value))} data-testid="effort-range-basic" aria-label="Effort" />
            <div class="quality-guidance"><span>1 = fastest</span><span>10 = smallest</span></div>
          </div>
          <div class="banner info" style="margin:9px 0 0">
            <span class="ic">i</span>
            <span>{settings.jpegLossless ? 'JPEG files use lossless transcode and ignore compression settings.' : 'JPEG files are included in the lossy reencode route.'}</span>
          </div>
        </div>
      </div>

      <div class="card">
        <h3>JPEG handling <span class="r">{routeCounts.Transcode + routeCounts.Reencode} files</span></h3>
        <div class="in policy" data-testid="jpeg-mode">
          <label class="opt" data-sel={settings.jpegLossless}>
            <input type="radio" name="jpeg-mode" checked={settings.jpegLossless} onchange={() => onSetJpegMode(true)} />
            <span><span class="ot">Transcode</span><span class="od">Bit-exactly reversible; the original JPEG can be reconstructed.</span></span>
          </label>
          <label class="opt" data-sel={!settings.jpegLossless}>
            <input type="radio" name="jpeg-mode" checked={!settings.jpegLossless} onchange={() => onSetJpegMode(false)} />
            <span><span class="ot">Reencode</span><span class="od">Uses distance and effort; not reversible.</span></span>
          </label>
          <div class="mini" style="padding:6px 8px 0;border-top:1px solid var(--line-soft);margin-top:4px">--lossless_jpeg={settings.jpegLossless ? 1 : 0}</div>
        </div>
      </div>

      <div class="card">
        <h3>Output</h3>
        <div class="in policy" data-testid="output-policy">
          <label class="opt" data-sel={settings.outputPolicy === 'alongside'}>
            <input type="radio" name="output" checked={settings.outputPolicy === 'alongside'} onchange={() => onSetOutputPolicy('alongside')} />
            <span><span class="ot">Alongside</span><span class="od">The original stays untouched.</span></span>
          </label>
          <label class="opt" data-sel={settings.outputPolicy === 'subfolder'}>
            <input type="radio" name="output" checked={settings.outputPolicy === 'subfolder'} onchange={() => onSetOutputPolicy('subfolder')} />
            <span><span class="ot">Into subfolder</span><span class="od">./jxl/ relative to the source.</span></span>
          </label>
          <label class="opt risk" data-sel={settings.outputPolicy === 'replace'}>
            <input type="radio" name="output" checked={settings.outputPolicy === 'replace'} onchange={() => onSetOutputPolicy('replace')} />
            <span><span class="ot">Replace, original to recycle bin</span><span class="od">Only after verification. Irreversible routes require confirmation.</span></span>
          </label>
          <label class="opt" data-sel={settings.embedSettings}>
            <input type="checkbox" checked={settings.embedSettings} onchange={(event) => onSetEmbedSettings((event.currentTarget as HTMLInputElement).checked)} data-testid="embed-settings" />
            <span><span class="ot">Settings in filename</span><span class="od">photo.d1.00-e7-cjxl0.11.1.jxl instead of photo.jxl</span></span>
          </label>
          <label class="opt" data-sel={settings.jxlInfoSidecar}>
            <input type="checkbox" checked={settings.jxlInfoSidecar} onchange={(event) => onSetJxlInfoSidecar((event.currentTarget as HTMLInputElement).checked)} data-testid="jxlinfo-sidecar" />
            <span><span class="ot">JXL info sidecar</span><span class="od">&lt;output&gt;.jxlinfo.txt next to the converted file, always overwritten</span></span>
          </label>
        </div>
      </div>
    </div>
  </div>
</div>
{#if files.length > 0}
  <div class="convertbar" data-testid="convertbar">
    <span class="mini">{files.length} files - {formatBytes(totalSize)}</span>
    <span class="spacer"></span>
    <button class="btn" data-testid="new-files" onclick={onClearAll}>Clear All</button>
    <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" data-testid="move-to-queue" onclick={onMoveToQueue} disabled={!canQueue}>Move {files.length} file{files.length === 1 ? '' : 's'} to queue</button>
  </div>
{/if}
