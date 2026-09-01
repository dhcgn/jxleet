<script lang="ts">
  import type { ConversionSummary, FilePreview, FileUpdate, ProgressUpdate, Status, ToolchainProgress, ToolchainStatus } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { compactPath, formatBytes, formatDelta, formatEta, formatRate, savedPct } from '../lib/format';
  import { effortNames } from '../lib/effort';
  import { routeClass, routeTitle } from '../lib/routes';
  import type { FileGroup, RouteMode } from '../lib/types';
  import QualitySliders from '../components/QualitySliders.svelte';
  import JxlInfoPanel from '../components/JxlInfoPanel.svelte';

  interface Props {
    files: FilePreview[];
    results: FileUpdate[];
    meta: { selection: string; output: string; error: string; loading: boolean };
    run: { busy: boolean; summary: ConversionSummary | null };
    progress: ProgressUpdate;
    settings: { distance: number; effort: number; jpegLossless: boolean; outputPolicy: string };
    routeMode: RouteMode;
    quality: number;
    outOfRange: boolean;
    presetName: string;
    appStatus: Status | null;
    tools: { status: ToolchainStatus | null; installing: boolean; progress: ToolchainProgress | null };
    canConvert: boolean;
    pendingCount: number;
    onOpenFile(): void;
    onOpenFolder(): void;
    onTogglePause(): void;
    onCancel(): void;
    onInstallToolchain(): void;
    onGoToPresets(): void;
    onSelectResult(result: FileUpdate): void;
    onClearAll(): void;
    onSetDistance(value: number): void;
    onSetQuality(quality: number): void;
    onSetEffort(value: number): void;
    onSetJpegMode(lossless: boolean): void;
    onSetOutputPolicy(policy: 'alongside' | 'subfolder' | 'replace'): void;
    onStart(): void;
  }
  let {
    files,
    results,
    meta,
    run,
    progress,
    settings,
    routeMode,
    quality,
    outOfRange,
    presetName,
    appStatus,
    tools,
    canConvert,
    pendingCount,
    onOpenFile,
    onOpenFolder,
    onTogglePause,
    onCancel,
    onInstallToolchain,
    onGoToPresets,
    onSelectResult,
    onClearAll,
    onSetDistance,
    onSetQuality,
    onSetEffort,
    onSetJpegMode,
    onSetOutputPolicy,
    onStart,
  }: Props = $props();

  let collapsedGroups = $state(new Set<string>());
  let resultByInput = $derived.by(() => {
    const map = new Map<string, FileUpdate>();
    for (const result of results) map.set(result.input, result);
    return map;
  });
  let selectedResult = $derived(results.find((result) => result.input === meta.selection) ?? null);
  let routeCounts = $derived.by(() => ({
    Transcode: files.filter((file) => file.route === 'Transcode').length,
    Reencode: files.filter((file) => file.route === 'Reencode').length,
    Encode: files.filter((file) => file.route === 'Encode').length,
    Skip: files.filter((file) => file.skip).length,
  }));
  let dominantRoute = $derived(
    routeCounts.Reencode >= routeCounts.Encode && routeCounts.Reencode >= routeCounts.Transcode
      ? 'Reencode'
      : routeCounts.Encode >= routeCounts.Transcode
        ? 'Encode'
        : 'Transcode',
  );
  let totalSize = $derived(files.reduce((total, file) => total + file.size, 0));
  let groups = $derived.by<FileGroup[]>(() => {
    const map = new Map<string, FileGroup>();
    for (const file of files) {
      const route = file.skip ? 'Skip' : file.route || 'pending';
      const key = `${route}|${file.format}|${file.skip ? file.reason : file.settings}|${file.flagsSet}`;
      let group = map.get(key);
      if (!group) {
        group = {
          key,
          format: file.format,
          route,
          skip: file.skip,
          reason: file.reason,
          settings: file.settings,
          flagsSet: file.flagsSet,
          files: [],
          sizeIn: 0,
          sizeOut: 0,
          sizeDoneIn: 0,
          hasResults: false,
        };
        map.set(key, group);
      }
      group.files.push(file);
      group.sizeIn += file.size;
      const result = resultByInput.get(file.path);
      if (result && !result.error && !result.skipped && !result.cancelled) {
        group.sizeOut += result.outputSize;
        group.sizeDoneIn += result.inputSize;
        group.hasResults = true;
      }
    }
    const order = (route: string) => (route === 'Transcode' ? 0 : route === 'Reencode' ? 1 : route === 'Encode' ? 2 : 3);
    return [...map.values()].sort((a, b) => order(a.route) - order(b.route));
  });
  let fileStatuses = $derived.by(() => {
    const map = new Map<string, string>();
    let runningLeft = run.busy ? progress.inFlight : 0;
    for (const file of files) {
      const result = resultByInput.get(file.path);
      if (result) {
        map.set(file.path, result.error ? 'failed' : result.skipped ? 'skipped' : result.cancelled ? 'cancelled' : 'done');
      } else if (run.busy && runningLeft > 0) {
        map.set(file.path, 'running');
        runningLeft -= 1;
      } else if (run.busy) {
        map.set(file.path, 'waiting');
      } else {
        map.set(file.path, '');
      }
    }
    return map;
  });

  function fileStatus(file: FilePreview): string {
    return fileStatuses.get(file.path) ?? '';
  }

  function toggleGroup(key: string): void {
    const next = new Set(collapsedGroups);
    if (next.has(key)) {
      next.delete(key);
    } else {
      next.add(key);
    }
    collapsedGroups = next;
  }
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
  {#if files.length === 0 && !run.busy}
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
  {#if run.busy}
    <div class="run-strip" data-testid="run-strip">
      <div class="run-head">
        <span class="badge b-reencode">{progress.paused ? 'Paused' : 'Converting'}</span>
        <span class="run-count">{progress.completed + progress.failed + progress.skipped} of {progress.total}</span>
        <span class="mini">{formatEta(progress.etaSeconds)} remaining - {formatRate(progress.throughput)}</span>
        <span class="spacer"></span>
        <button class="btn" onclick={onTogglePause}>{progress.paused ? 'Resume' : 'Pause'}</button>
        <button class="btn danger" data-testid="cancel" onclick={onCancel}>Cancel</button>
      </div>
      <div class="bar"><i style={`width:${Math.min(100, progress.percent)}%`}></i></div>
    </div>
  {/if}
  {#if presetName === ''}
    <div class="banner info" style="margin-bottom:12px"><span class="ic">i</span><span>Files are selected. Select a preset in the toolbar to classify their routes.</span></div>
  {/if}
  {#if files.length === 0}
    <div class="empty">Preparing the queue...</div>
  {:else}
  <div class="cols">
    <div class="groups-col">
      {#each groups as group (group.key)}
        <div class="card group" data-testid={`group-${group.route.toLowerCase()}`}>
          <button type="button" class="group-head" aria-expanded={!collapsedGroups.has(group.key)} onclick={() => toggleGroup(group.key)}>
            <span class={`badge ${routeClass(group.route)}`}>{group.skip ? 'skip' : group.format || 'file'}</span>
            <span class="group-title">{group.route === 'pending' ? 'Select a preset' : routeTitle(group.route)}</span>
            <span class="mini">{group.files.length} files - {formatBytes(group.sizeIn)}</span>
            <span class="spacer"></span>
            {#if group.skip}
              <span class="mini">{group.reason || 'skipped'}</span>
            {:else}
              <span class="mono-mini" title={group.flagsSet ? `${group.settings} + extra flags` : group.settings}>{group.settings || '-'}</span>
              {#if group.flagsSet}<span class="flag-chip" title="Extra cjxl flags applied">+flags</span>{/if}
              {#if group.hasResults && group.sizeDoneIn > 0}
                <span class="delta-chip" class:neg={savedPct(group.sizeDoneIn, group.sizeOut) < 0}>{formatDelta(group.sizeDoneIn, group.sizeOut)}</span>
              {/if}
            {/if}
            <svg class="chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m6 4 4 4-4 4"></path></svg>
          </button>
          {#if !collapsedGroups.has(group.key)}
            <table class="files group-files" data-testid="file-table">
              <colgroup>
                <col class="gf-file" />
                <col class="gf-hug" />
                <col class="gf-hug" />
                <col class="gf-status" />
              </colgroup>
              <thead><tr><th>File</th><th style="text-align:right">Size</th><th style="text-align:right">JXL</th><th>Result</th></tr></thead>
              <tbody>
                {#each group.files as file (file.path)}
                  {@const result = resultByInput.get(file.path)}
                  {@const failed = result != null && result.error !== ''}
                  {@const inspectable = run.summary != null && !run.busy && result != null && !failed && !result.skipped && !result.cancelled}
                  <tr
                    class:selected={result != null && meta.selection === result.input}
                    class:clickable={inspectable}
                    onclick={() => { if (inspectable && result) void onSelectResult(result); }}
                  >
                    <td class="fn" title={file.path}>{file.name}</td>
                    <td class="num">{formatBytes(file.size)}</td>
                    <td class="num">{result && !failed && !result.skipped && !result.cancelled ? formatBytes(result.outputSize) : '-'}</td>
                    <td class="status-cell" class:success={result != null && !failed && !result.skipped && !result.cancelled} class:error={failed}>
                      {#if result}
                        {failed ? (result.error || 'failed') : result.skipped ? (result.skipReason || 'skipped') : result.cancelled ? 'cancelled' : formatDelta(result.inputSize, result.outputSize)}
                      {:else if group.skip}
                        {file.reason || 'skipped'}
                      {:else if run.busy}
                        {fileStatus(file)}
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
      {/each}
      <div class="mini" style="padding:0 2px">Drop more files or folders anywhere in the window to add them.</div>
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
        </div>
      </div>

      {#if run.summary && !run.busy}
        <div class="card">
          <h3>Output summary</h3>
          <div class="in kv">
            <div><span>Converted</span><span>{run.summary.completed}{#if run.summary.failed > 0} - {run.summary.failed} failed{/if}{#if run.summary.skipped > 0} - {run.summary.skipped} skipped{/if}</span></div>
            <div><span>Bytes</span><span>{formatBytes(run.summary.bytesIn)} -&gt; {formatBytes(run.summary.bytesOut)}</span></div>
            <div><span>Saved</span><span class="success">{formatDelta(run.summary.bytesIn, run.summary.bytesOut)}</span></div>
          </div>
        </div>
        <JxlInfoPanel
          label={selectedResult ? (selectedResult.output ? compactPath(selectedResult.output, 34) : 'unavailable') : 'select a converted file'}
          emptyText="Select a converted file to inspect its JPEG XL metadata."
          hasSelection={selectedResult != null}
          loading={meta.loading}
          error={meta.error}
          output={meta.output}
        />
      {/if}
    </div>
  </div>
  {/if}
  {/if}
</div>
{#if files.length > 0}
  <div class="convertbar" data-testid="convertbar">
    {#if run.summary && !run.busy}
      <span class:error={run.summary.failed > 0} class:success={run.summary.failed === 0}>{run.summary.completed} converted{#if run.summary.failed > 0} - {run.summary.failed} failed{/if}{#if run.summary.skipped > 0} - {run.summary.skipped} skipped{/if}</span>
      <span class="mono-mini">{formatBytes(run.summary.bytesIn)} -&gt; {formatBytes(run.summary.bytesOut)}</span>
      <span class="delta-chip" class:neg={savedPct(run.summary.bytesIn, run.summary.bytesOut) < 0}>{formatDelta(run.summary.bytesIn, run.summary.bytesOut)}</span>
    {:else}
      <span class="mini">{files.length} files - {formatBytes(totalSize)}</span>
    {/if}
    <span class="spacer"></span>
    {#if !run.busy}<button class="btn" data-testid="new-files" onclick={onClearAll}>Clear All</button>{/if}
    <button class="btn primary convert-action" style={`background:var(--p-${dominantRoute === 'Transcode' ? 'transcode' : dominantRoute === 'Encode' ? 'encode' : 'reencode'});padding:11px`} data-testid="start-convert" onclick={onStart} disabled={!canConvert}>Convert {pendingCount || files.length} files</button>
  </div>
{/if}
