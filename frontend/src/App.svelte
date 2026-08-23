<script lang="ts">
  import { onMount } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { Service } from '../bindings/github.com/dhcgn/jxleet/internal/app';
  import type {
    Bindings,
    ConversionOptions,
    ConversionSummary,
    FilePreview,
    FileUpdate,
    PresetSummary,
    ProgressUpdate,
    Status,
    ToolchainStatus,
  } from '../bindings/github.com/dhcgn/jxleet/internal/app';

  type View = 'drop' | 'basic' | 'expert' | 'running' | 'done' | 'tools' | 'automatic' | 'presets';
  type QualityUnit = 'distance' | 'quality';
  type RouteMode = 'lossy' | 'lossless';

  const effortNames = [
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

  const effortTools = [
    { name: 'Modular tree search', from: 3, lossy: false, lossless: true },
    { name: 'Palette detection', from: 2, lossy: false, lossless: true },
    { name: 'Delta palette', from: 6, lossy: false, lossless: true },
    { name: 'Weighted predictor', from: 4, lossy: false, lossless: true },
    { name: 'LZ77 in entropy coder', from: 2, lossy: true, lossless: true },
    { name: 'Context clustering', from: 5, lossy: true, lossless: true },
    { name: 'Adaptive quantisation', from: 3, lossy: true, lossless: false },
    { name: 'Chroma from luma', from: 4, lossy: true, lossless: false },
    { name: 'Patches / reference frames', from: 5, lossy: true, lossless: true },
    { name: 'Dots and splines detection', from: 6, lossy: true, lossless: false },
    { name: 'Error diffusion', from: 7, lossy: true, lossless: false },
    { name: 'Extended block size search', from: 8, lossy: true, lossless: false },
    { name: 'Full heuristic search', from: 9, lossy: true, lossless: true },
  ];

  let view = $state<View>('drop');
  let mode = $state<'basic' | 'expert'>('basic');
  let routeMode = $state<RouteMode>('lossy');
  let qualityUnit = $state<QualityUnit>('distance');
  let presetName = $state('');
  let selectedPreset = $state('');
  let presets = $state<PresetSummary[]>([]);
  let bindings = $state<Bindings>({ gui: '', cli: '', contextMenu: '' });
  let inputPaths = $state<string[]>([]);
  let files = $state<FilePreview[]>([]);
  let results = $state<FileUpdate[]>([]);
  let summary = $state<ConversionSummary | null>(null);
  let progress = $state<ProgressUpdate>({
    total: 0,
    completed: 0,
    failed: 0,
    skipped: 0,
    inFlight: 0,
    bytesTotal: 0,
    bytesDone: 0,
    throughput: 0,
    etaSeconds: 0,
    coalesced: 0,
    paused: false,
    percent: 0,
  });
  let appStatus = $state<Status | null>(null);
  let toolchain = $state<ToolchainStatus | null>(null);
  let toolchainError = $state('');
  let contextMenuRegistered = $state(false);
  let errorMessage = $state('');
  let loaded = $state(false);
  let busy = $state(false);
  let dragging = $state(false);
  let distance = $state(1.0);
  let effort = $state(7);
  let jpegLossless = $state(true);
  let outputPolicy = $state('alongside');
  let processes = $state(2);
  let threads = $state(8);

  let routeCounts = $derived.by(() => ({
    Transcode: files.filter((file) => file.route === 'Transcode').length,
    Reencode: files.filter((file) => file.route === 'Reencode').length,
    Encode: files.filter((file) => file.route === 'Encode').length,
    Skip: files.filter((file) => file.skip).length,
  }));
  let totalSize = $derived(files.reduce((total, file) => total + file.size, 0));
  let dominantRoute = $derived(
    routeCounts.Reencode >= routeCounts.Encode && routeCounts.Reencode >= routeCounts.Transcode
      ? 'Reencode'
      : routeCounts.Encode >= routeCounts.Transcode
        ? 'Encode'
        : 'Transcode',
  );
  let quality = $derived(Math.round(qualityFromDistance(distance)));
  let hasFiles = $derived(files.length > 0);

  onMount(() => {
    const offFiles = Events.On('files', (event: any) => {
      const incoming = Array.isArray(event?.data) ? event.data : [];
      if (incoming.length === 0) return;
      const wasRunning = view === 'running' || view === 'automatic';
      acceptPaths(incoming);
      if (wasRunning) view = 'automatic';
    });
    const offPreset = Events.On('preset', (event: any) => {
      const selected = String(event?.data ?? '');
      if (selected) {
        presetName = selected;
        void refreshPreview();
      }
    });
    const offProgress = Events.On('progress', (event: any) => {
      if (event?.data) {
        progress = event.data as ProgressUpdate;
        if (view !== 'automatic') view = 'running';
      }
    });
    const offFile = Events.On('conversion-file', (event: any) => {
      if (event?.data) {
        results = [...results, event.data as FileUpdate];
      }
    });
    const offDone = Events.On('conversion-done', (event: any) => {
      summary = event?.data as ConversionSummary;
      progress = { ...progress, paused: false, percent: 100 };
      view = 'done';
      busy = false;
    });
    const offError = Events.On('conversion-error', (event: any) => {
      errorMessage = String(event?.data ?? 'Conversion error');
    });

    void load();
    return () => {
      offFiles();
      offPreset();
      offProgress();
      offFile();
      offDone();
      offError();
    };
  });

  async function load(): Promise<void> {
    try {
      appStatus = await Service.GetStatus();
      bindings = await Service.GetBindings();
      presets = (await Service.ListPresets()) ?? [];
      presetName = bindings.gui;
      try {
        toolchain = await Service.GetToolchainStatus();
      } catch (error) {
        toolchainError = errorText(error);
      }
      try {
        contextMenuRegistered = await Service.ContextMenuRegistered();
      } catch (error) {
        errorMessage = errorText(error);
      }
      const pending = (await Service.TakePending()) ?? [];
      const pendingPreset = await Service.TakePendingPreset();
      const activePreset = await Service.GetActivePreset();
      if (pendingPreset) presetName = pendingPreset;
      if (activePreset) presetName = activePreset;
      if (pending.length > 0) {
        acceptPaths(pending);
        await refreshPreview();
      }
      const runningProgress = await Service.GetProgress();
      if (runningProgress.total > 0) {
        progress = runningProgress;
        view = runningProgress.coalesced > 1 ? 'automatic' : 'running';
        busy = true;
      }
    } catch (error) {
      errorMessage = errorText(error);
    } finally {
      loaded = true;
    }
  }

  function currentOptions(): ConversionOptions {
    return {
      preset: presetName,
      processes,
      threads,
      jpegMode: jpegLossless ? 'transcode' : 'reencode',
      distance,
      useDistance: true,
      effort,
      useEffort: true,
      outputPolicy,
    };
  }

  async function refreshPreview(): Promise<void> {
    if (inputPaths.length === 0 || presetName === '') {
      files = [];
      return;
    }
    try {
      files = (await Service.PreviewPaths(inputPaths, currentOptions())) ?? [];
      errorMessage = '';
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  function acceptPaths(paths: string[]): void {
    const next = [...inputPaths, ...paths.filter((path) => path.trim() !== '')];
    inputPaths = [...new Set(next)];
    errorMessage = '';
    view = 'basic';
    void refreshPreview();
  }

  async function browse(): Promise<void> {
    try {
      const paths = (await Service.OpenFiles()) ?? [];
      acceptPaths(paths);
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  function handleDrop(event: DragEvent): void {
    event.preventDefault();
    dragging = false;
    const paths = Array.from(event.dataTransfer?.files ?? [])
      .map((file) => (file as File & { path?: string }).path ?? '')
      .filter(Boolean);
    if (paths.length === 0) {
      errorMessage = 'The WebView did not expose local paths. Use Browse to select files or folders.';
      return;
    }
    acceptPaths(paths);
  }

  async function startConversion(): Promise<void> {
    if (inputPaths.length === 0) {
      errorMessage = 'Drop or browse for at least one file or folder first.';
      return;
    }
    if (presetName === '') {
      errorMessage = 'Select or bind a preset before converting.';
      return;
    }
    if (outputPolicy === 'replace') {
      const irreversible = files.filter((file) => file.route === 'Reencode' || (file.route === 'Encode' && distance > 0)).length;
      if (irreversible > 0 && !window.confirm(`Replace the originals for ${irreversible} irreversible file${irreversible === 1 ? '' : 's'}? They will be sent to the recycle bin after verification.`)) {
        return;
      }
    }
    busy = true;
    errorMessage = '';
    results = [];
    summary = null;
    view = 'running';
    try {
      await Service.StartConversion(inputPaths, currentOptions());
    } catch (error) {
      busy = false;
      view = 'basic';
      errorMessage = errorText(error);
    }
  }

  async function togglePause(): Promise<void> {
    try {
      if (progress.paused) {
        await Service.ResumeConversion();
      } else {
        await Service.PauseConversion();
      }
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function cancelConversion(): Promise<void> {
    try {
      await Service.CancelConversion();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function refreshToolchain(): Promise<void> {
    toolchainError = '';
    try {
      toolchain = await Service.GetToolchainStatus();
    } catch (error) {
      toolchainError = errorText(error);
    }
  }

  async function installToolchain(): Promise<void> {
    busy = true;
    toolchainError = '';
    try {
      await Service.InstallLatestToolchain();
      await refreshToolchain();
    } catch (error) {
      toolchainError = errorText(error);
    } finally {
      busy = false;
    }
  }

  async function createPreset(): Promise<void> {
    const name = window.prompt('Preset name');
    if (!name) return;
    const description = window.prompt('Description', '') ?? '';
    try {
      await Service.CreatePreset(name, description);
      await refreshPresets();
      presetName = name.trim();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function duplicatePreset(): Promise<void> {
    if (!selectedPreset) return;
    const name = window.prompt('Duplicate as', `${selectedPreset}-copy`);
    if (!name) return;
    try {
      await Service.DuplicatePreset(selectedPreset, name);
      await refreshPresets();
      selectedPreset = name.trim();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function renamePreset(): Promise<void> {
    if (!selectedPreset) return;
    const name = window.prompt('Rename preset', selectedPreset);
    if (!name || name.trim() === selectedPreset) return;
    try {
      await Service.RenamePreset(selectedPreset, name);
      if (presetName === selectedPreset) presetName = name.trim();
      await refreshPresets();
      selectedPreset = name.trim();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function deletePreset(): Promise<void> {
    if (!selectedPreset || !window.confirm(`Delete "${selectedPreset}"?`)) return;
    try {
      await Service.DeletePreset(selectedPreset);
      if (presetName === selectedPreset) presetName = '';
      selectedPreset = '';
      await refreshPresets();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function refreshPresets(): Promise<void> {
    presets = (await Service.ListPresets()) ?? [];
  }

  async function setBinding(entryPoint: string, value: string): Promise<void> {
    if (!value) return;
    try {
      await Service.SetBinding(entryPoint, value);
      bindings = { ...bindings, [bindingKey(entryPoint)]: value };
      if (entryPoint === 'gui') {
        presetName = value;
        await refreshPreview();
      }
      appStatus = await Service.GetStatus();
      if (entryPoint === 'contextmenu') {
        contextMenuRegistered = await Service.ContextMenuRegistered();
      }
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function registerContextMenu(): Promise<void> {
    try {
      await Service.RegisterContextMenu();
      contextMenuRegistered = await Service.ContextMenuRegistered();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function unregisterContextMenu(): Promise<void> {
    try {
      await Service.UnregisterContextMenu();
      contextMenuRegistered = false;
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  function bindingKey(entryPoint: string): keyof Bindings {
    if (entryPoint === 'gui') return 'gui';
    if (entryPoint === 'cli') return 'cli';
    return 'contextMenu';
  }

  function resetDrop(): void {
    inputPaths = [];
    files = [];
    results = [];
    summary = null;
    progress = { ...progress, total: 0, completed: 0, failed: 0, skipped: 0, percent: 0 };
    view = 'drop';
  }

  function routeClass(route: string): string {
    if (route === 'Transcode') return 'b-transcode';
    if (route === 'Reencode') return 'b-reencode';
    if (route === 'Encode') return 'b-encode';
    return 'b-skip';
  }

  function routeTitle(route: string): string {
    if (route === 'Transcode') return 'JPEG transcode';
    if (route === 'Reencode') return 'JXL reencode';
    if (route === 'Encode') return 'Pixel encode';
    return 'Skipped';
  }

  function formatBytes(value: number): string {
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

  function formatEta(seconds: number): string {
    if (!seconds || seconds < 1) return '--:--';
    const minutes = Math.floor(seconds / 60);
    const remainder = Math.floor(seconds % 60);
    return `${minutes}:${remainder.toString().padStart(2, '0')}`;
  }

  function formatRate(value: number): string {
    return value > 0 ? `${formatBytes(value)}/s` : '--';
  }

  function qualityFromDistance(value: number): number {
    if (value <= 0) return 100;
    if (value <= 6.4) return Math.max(0, Math.min(100, 100 - (value - 0.1) / 0.09));
    const a = 53 / 3000;
    const b = -23 / 20;
    const discriminant = b * b - 4 * a * (25 - value);
    return Math.max(0, Math.min(100, (-b - Math.sqrt(Math.max(0, discriminant))) / (2 * a)));
  }

  function setDistance(event: Event): void {
    distance = Number((event.currentTarget as HTMLInputElement).value) / 10;
  }

  function setEffort(event: Event): void {
    effort = Number((event.currentTarget as HTMLInputElement).value);
  }

  function commandPreview(): string {
    const flags = [`--distance=${distance}`, `--effort=${effort}`, `--num_threads=${threads}`];
    if (jpegLossless) flags.push('--lossless_jpeg=1');
    else flags.push('--lossless_jpeg=0');
    return `cjxl ${flags.join(' ')} input output.jxl`;
  }

  function errorText(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
</script>

<svelte:head>
  <title>jxleet</title>
</svelte:head>

<svelte:window onkeydown={(event) => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'o') {
    event.preventDefault();
    void browse();
  }
}} />

<main class:running={view === 'running' || view === 'automatic'} class:narrow={view === 'automatic'} class="app">
  <div class="titlebar">
    <span class="name">JXLEET{view === 'drop' ? '' : ` - ${view.toUpperCase()}`}</span>
    <span class="spacer"></span>
    <span class="wctl" aria-hidden="true">-</span>
    <span class="wctl" aria-hidden="true">[]</span>
    <span class="wctl close" aria-hidden="true">x</span>
  </div>

  <div class="toolbar">
    <div class="seg" aria-label="Editor mode">
      <button aria-pressed={mode === 'basic'} onclick={() => { mode = 'basic'; if (view === 'expert') view = 'basic'; }}>Basic</button>
      <button aria-pressed={mode === 'expert'} onclick={() => { mode = 'expert'; view = 'expert'; }}>Expert</button>
    </div>
    <div class="field">
      <label for="preset-select">Preset</label>
      <select id="preset-select" bind:value={presetName} onchange={() => void refreshPreview()} disabled={presets.length === 0}>
        <option value="">Select a preset</option>
        {#each presets as preset}
          <option value={preset.name}>{preset.name}</option>
        {/each}
      </select>
    </div>
    <button class="btn ghost" onclick={() => { view = 'presets'; mode = 'basic'; }}>Presets</button>
    <button class="btn ghost" onclick={() => { view = 'tools'; mode = 'basic'; }}>Tools</button>
    <span class="spacer"></span>
    {#if view === 'done'}
      <button class="btn" onclick={resetDrop}>New drop</button>
    {:else if view === 'presets' || view === 'tools'}
      <button class="btn" onclick={() => { view = hasFiles ? 'basic' : 'drop'; }}>Back</button>
    {/if}
  </div>

  {#if errorMessage}
    <div class="banner warn" role="alert"><span class="ic">!</span><span>{errorMessage}</span></div>
  {/if}

  {#if !loaded}
    <div class="body"><div class="empty">Loading jxleet...</div></div>
  {:else if view === 'drop'}
    <div class="body">
      {#if toolchain?.needsInstall}
        <div class="banner warn">
          <span class="ic">!</span>
          <span><b>libjxl is not installed.</b> Install the managed cjxl/djxl/jxlinfo toolchain before converting.</span>
          <button class="btn primary" style="margin-left:auto;background:var(--p-encode)" onclick={() => void installToolchain()} disabled={busy}>Install</button>
        </div>
      {/if}
      {#if appStatus && !appStatus.ready}
        <div class="banner info">
          <span class="ic">i</span>
          <span>Bind a preset for each entry point before automated runs.</span>
          <button class="btn ghost" style="margin-left:auto" onclick={() => { view = 'presets'; }}>Set bindings</button>
        </div>
      {/if}
      <div
        class:dragging
        class="drop"
        role="button"
        tabindex="0"
        aria-label="Drop files or folders"
        ondragover={(event) => { event.preventDefault(); dragging = true; }}
        ondragleave={() => { dragging = false; }}
        ondrop={handleDrop}
        onclick={() => void browse()}
        onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') void browse(); }}
      >
        <div class="big">Drop files or folders here</div>
        <div class="sub">jxleet detects the input format and chooses the route. Unsupported files are skipped and reported.</div>
        <div style="display:flex;gap:14px;justify-content:center;margin-top:6px;flex-wrap:wrap">
          <span class="badge b-transcode">JPEG - transcode</span>
          <span class="badge b-reencode">JPEG / JXL - reencode</span>
          <span class="badge b-encode">Pixel - encode</span>
        </div>
        <div class="sub" style="margin-top:10px">or <kbd>Ctrl</kbd> + <kbd>O</kbd> to browse</div>
        <button class="btn primary" style="margin:4px auto 0;background:var(--p-encode)" onclick={(event) => { event.stopPropagation(); void browse(); }}>Browse</button>
      </div>
    </div>
  {:else if view === 'basic'}
    <div class="body">
      <div class="paths">
        <div class="path p1" class:off={routeCounts.Transcode === 0}>
          <div class="t">JPEG transcode</div>
          <div class="n">{routeCounts.Transcode}</div>
          <div class="d">Lossless and byte-reversible. Distance does not apply.</div>
        </div>
        <div class="path p2" class:off={routeCounts.Reencode === 0}>
          <div class="t">JXL reencode</div>
          <div class="n">{routeCounts.Reencode}</div>
          <div class="d">Decoded and encoded again with the selected settings.</div>
        </div>
        <div class="path p3" class:off={routeCounts.Encode === 0}>
          <div class="t">Pixel encode</div>
          <div class="n">{routeCounts.Encode}</div>
          <div class="d">PNG, GIF, EXR and NetPBM formats; lossless at distance 0.</div>
        </div>
      </div>

      <div class="cols">
        <div class="card">
          <h3>Files <span class="r">{files.length} files - {formatBytes(totalSize)}</span></h3>
          {#if files.length === 0}
            <div class="empty">Choose a preset to classify the selected paths.</div>
          {:else}
            <table class="files" data-testid="file-table">
              <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Size</th></tr></thead>
              <tbody>
                {#each files as file}
                  <tr>
                    <td class="fn" title={file.path}>{file.name}</td>
                    <td><span class={`badge ${routeClass(file.route)}`}>{file.skip ? 'skip' : file.route}</span></td>
                    <td class="num">{formatBytes(file.size)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>

        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Compression <span class="r">applies to non-transcoded files</span></h3>
            <div class="in">
              <div class="seg" style="width:100%;margin-bottom:9px">
                <button aria-pressed={qualityUnit === 'distance'} style="flex:1" onclick={() => qualityUnit = 'distance'}>Distance</button>
                <button aria-pressed={qualityUnit === 'quality'} style="flex:1" onclick={() => qualityUnit = 'quality'}>Quality</button>
              </div>
              <div style="display:flex;align-items:baseline;gap:8px;margin-bottom:2px">
                <span style="font:600 22px/1 var(--mono)">{qualityUnit === 'distance' ? distance.toFixed(1) : quality}</span>
                <span class="mini">{qualityUnit === 'distance' ? `Quality ${quality}` : `Distance ${distance.toFixed(1)}`}</span>
                <span class="mini" style="margin-left:auto">0 = lossless</span>
              </div>
              {#if qualityUnit === 'distance'}
                <input type="range" min="0" max="150" value={distance * 10} oninput={setDistance} aria-label="Distance" />
              {:else}
                <input type="range" min="0" max="100" value={quality} oninput={(event) => distance = Number((event.currentTarget as HTMLInputElement).value) === 100 ? 0 : 0.1 + (100 - Number((event.currentTarget as HTMLInputElement).value)) * 0.09} aria-label="Quality" />
              {/if}
              <div class="row" style="border-top:1px solid var(--line-soft);margin-top:6px">
                <span class="k">Effort</span><span class="v">{effort} - {effortNames[effort - 1]}</span>
              </div>
              <div class="banner info" style="margin:9px 0 0">
                <span class="ic">i</span>
                <span>{jpegLossless ? 'JPEG files use lossless transcode and ignore compression settings.' : 'JPEG files are included in the lossy reencode route.'}</span>
              </div>
            </div>
          </div>

          <div class="card">
            <h3>JPEG handling <span class="r">{routeCounts.Transcode + routeCounts.Reencode} files</span></h3>
            <div class="in policy" data-testid="jpeg-mode">
              <label class="opt" data-sel={jpegLossless}>
                <input type="radio" name="jpeg-mode" checked={jpegLossless} onchange={() => { jpegLossless = true; void refreshPreview(); }} />
                <span><span class="ot">Transcode</span><span class="od">Bit-exactly reversible; the original JPEG can be reconstructed.</span></span>
              </label>
              <label class="opt" data-sel={!jpegLossless}>
                <input type="radio" name="jpeg-mode" checked={!jpegLossless} onchange={() => { jpegLossless = false; void refreshPreview(); }} />
                <span><span class="ot">Reencode</span><span class="od">Uses distance and effort; not reversible.</span></span>
              </label>
              <div class="mini" style="padding:6px 8px 0;border-top:1px solid var(--line-soft);margin-top:4px">--lossless_jpeg={jpegLossless ? 1 : 0}</div>
            </div>
          </div>

          <div class="card">
            <h3>Output</h3>
            <div class="in policy" data-testid="output-policy">
              <label class="opt" data-sel={outputPolicy === 'alongside'}>
                <input type="radio" name="output" checked={outputPolicy === 'alongside'} onchange={() => outputPolicy = 'alongside'} />
                <span><span class="ot">Alongside</span><span class="od">The original stays untouched.</span></span>
              </label>
              <label class="opt" data-sel={outputPolicy === 'subfolder'}>
                <input type="radio" name="output" checked={outputPolicy === 'subfolder'} onchange={() => outputPolicy = 'subfolder'} />
                <span><span class="ot">Into subfolder</span><span class="od">./jxl/ relative to the source.</span></span>
              </label>
              <label class="opt risk" data-sel={outputPolicy === 'replace'}>
                <input type="radio" name="output" checked={outputPolicy === 'replace'} onchange={() => outputPolicy = 'replace'} />
                <span><span class="ot">Replace, original to recycle bin</span><span class="od">Only after verification. Irreversible routes require confirmation.</span></span>
              </label>
            </div>
          </div>

          <button class="btn primary" style={`background:var(--p-${dominantRoute === 'Transcode' ? 'transcode' : dominantRoute === 'Encode' ? 'encode' : 'reencode'});padding:11px`} data-testid="start-convert" onclick={() => void startConversion()} disabled={!hasFiles || busy}>
            Convert {files.length} files
          </button>
        </div>
      </div>
    </div>
  {:else if view === 'expert'}
    <div class="body">
      <div class="toolbar" style="margin:-12px -12px 12px">
        <div class="field"><span class="field-label">Route</span>
          <div class="seg">
            <button aria-pressed={routeMode === 'lossy'} onclick={() => routeMode = 'lossy'}>Lossy</button>
            <button aria-pressed={routeMode === 'lossless'} onclick={() => routeMode = 'lossless'}>Lossless</button>
          </div>
        </div>
        <span class="spacer"></span>
        <button class="btn ghost" onclick={() => { distance = 1; effort = 7; jpegLossless = true; }}>Reset to preset</button>
      </div>
      <div class="cols wide-right">
        <div class="card">
          <h3>Effort <span class="r">what this level adds</span></h3>
          <div class="in">
            <div class="ladder-head">
              <span class="lvl">{effort}</span>
              <span class="nm">{effortNames[effort - 1]}</span>
              <span class="hint">{effort === 7 ? 'cjxl default' : effort >= 9 ? 'noticeably slower' : effort <= 3 ? 'very fast, larger file' : ''}</span>
            </div>
            <input type="range" min="1" max="10" value={effort} oninput={setEffort} data-testid="effort-range" aria-label="Effort" />
            <div class="ticks">
              {#each effortNames as name, index}
                <span class:on={index + 1 === effort}>{index + 1}</span>
              {/each}
            </div>
            <table class="grid" data-testid="effort-grid">
              <thead><tr><th>Tool</th><th class="c" colspan="10" style="text-align:center">1 - 10</th><th class="c">L</th><th class="c">LL</th></tr></thead>
              <tbody>
                {#each effortTools as tool}
                  {@const applicable = routeMode === 'lossy' ? tool.lossy : tool.lossless}
                  <tr class:na={!applicable}>
                    <td class="nm">{tool.name}</td>
                    {#each effortNames as _, index}
                      <td class:colhi={index + 1 === effort} class="cell"><span class:on={applicable && index + 1 >= tool.from} class:cur={applicable && index + 1 === effort} class="dot"></span></td>
                    {/each}
                    <td class="mode">{tool.lossy ? 'yes' : '-'}</td>
                    <td class="mode">{tool.lossless ? 'yes' : '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
            <div class="cmd" data-testid="cmd-preview">{commandPreview()}</div>
          </div>
        </div>

        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Quality</h3>
            <div class="in">
              <div class="seg" style="width:100%;margin-bottom:9px">
                <button aria-pressed={qualityUnit === 'distance'} style="flex:1" data-testid="unit-distance" onclick={() => qualityUnit = 'distance'}>Distance</button>
                <button aria-pressed={qualityUnit === 'quality'} style="flex:1" data-testid="unit-quality" onclick={() => qualityUnit = 'quality'}>Quality</button>
              </div>
              <div style="display:flex;align-items:baseline;gap:8px">
                <span style="font:600 22px/1 var(--mono)">{qualityUnit === 'distance' ? distance.toFixed(1) : quality}</span>
                <span class="mini">same stored quantity</span>
              </div>
              <input type="range" min="0" max="150" value={distance * 10} oninput={setDistance} aria-label="Distance" />
              <div class="mini" style="display:flex;justify-content:space-between"><span>0 lossless</span><span>3.0 visible</span></div>
              <div class="banner info" style="margin:10px 0 0"><span class="ic">i</span><span>Distance and quality change only the display, never the stored setting.</span></div>
            </div>
          </div>
          <div class="card">
            <h3>Further flags</h3>
            <div class="in">
              <div class="row"><span class="k">JPEG input</span><span class="v">{jpegLossless ? '1 - transcode' : '0 - reencode'}</span></div>
              <div class="row"><span class="k">Modular</span><span class="v">auto</span></div>
              <div class="row"><span class="k">Progressive</span><span class="v">off</span></div>
              <div class="row"><span class="k">Brotli effort</span><span class="v">9</span></div>
              <div class="row"><span class="k">Threads per process</span><span class="v">{threads}</span></div>
              <div class="row"><span class="k">Parallel processes</span><span class="v">{processes}</span></div>
            </div>
          </div>
          <button class="btn primary" style="background:var(--p-encode);padding:11px" onclick={() => void startConversion()} disabled={!hasFiles || busy}>Convert {files.length} files</button>
        </div>
      </div>
    </div>
  {:else if view === 'running' || view === 'automatic'}
    <div class="body">
      <div class="toolbar" style="margin:-12px -12px 12px">
        <span class="badge b-reencode">Running</span>
        <span class="mini">{presetName || 'preset'} - {processes} processes - {threads} threads</span>
        <span class="spacer"></span>
        <button class="btn" onclick={() => void togglePause()}>{progress.paused ? 'Resume' : 'Pause'}</button>
        <button class="btn danger" data-testid="cancel" onclick={() => void cancelConversion()}>Cancel</button>
      </div>
      {#if progress.coalesced > 1}
        <div class="banner info"><span class="ic">+</span><span><b>{progress.coalesced} invocations coalesced into one run.</b> Further paths are handed to this window.</span></div>
      {/if}
      <div class="cols">
        <div class="card">
          <h3>Progress</h3>
          <div class="in">
            <div class="big-prog">
              <span class="pc">{Math.round(progress.percent)}<span style="font-size:18px;color:var(--ink3)">%</span></span>
              <span class="of">{progress.completed + progress.failed + progress.skipped} of {progress.total}</span>
              <span class="eta"><b>{formatEta(progress.etaSeconds)}</b><span>remaining</span></span>
            </div>
            <div class="bar"><i style={`width:${Math.min(100, progress.percent)}%`}></i></div>
            <div style="display:flex;justify-content:space-between;margin-top:6px" class="mini">
              <span>{formatBytes(progress.bytesDone)} of {formatBytes(progress.bytesTotal)}</span>
              <span>{formatRate(progress.throughput)}</span>
            </div>
            <div style="margin-top:14px">
              <div class="mini" style="margin-bottom:6px">Completed</div>
              {#each results.slice(-4).reverse() as result}
                <div class="row" style="padding:5px 0"><span class="k" style="font-family:var(--mono);color:var(--ink)">{result.input.split(/[\\/]/).pop()}</span><span class="v">{result.error ? 'failed' : result.skipped ? 'skipped' : 'done'}</span></div>
              {/each}
            </div>
          </div>
        </div>
        <div class="card">
          <h3>Queue <span class="r">{Math.max(0, progress.total - progress.completed - progress.failed - progress.skipped)} open</span></h3>
          <div class="in q" data-testid="queue">
            {#each files.slice(0, 12) as file, index}
              <div>{file.name} <span class="st">{index < progress.completed ? 'done' : index < progress.completed + progress.inFlight ? 'running' : 'waiting'}</span></div>
            {/each}
            {#if files.length > 12}<div class="st" style="padding-top:6px">... {files.length - 12} more</div>{/if}
          </div>
        </div>
      </div>
    </div>
  {:else if view === 'done'}
    <div class="body">
      <div class="toolbar" style="margin:-12px -12px 12px">
        <span class:success={!summary?.failed} class:error={Boolean(summary?.failed)}>{summary?.completed ?? 0} converted - {summary?.failed ?? 0} failed</span>
        <span class="mini">{formatBytes(summary?.bytesIn ?? 0)} -> {formatBytes(summary?.bytesOut ?? 0)}</span>
        <span class="spacer"></span>
        <button class="btn ghost" onclick={() => { view = 'tools'; }}>Tools</button>
        <button class="btn" onclick={resetDrop}>New drop</button>
      </div>
      <div class="cols wide-right">
        <div class="card">
          <h3>Result <span class="r">{results.length} files</span></h3>
          {#if results.length === 0}
            <div class="empty">No file results were reported.</div>
          {:else}
            <table class="files" data-testid="result-table">
              <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Before</th><th style="text-align:right">After</th><th>Status</th></tr></thead>
              <tbody>
                {#each results as result}
                  <tr>
                    <td class="fn" title={result.input}>{result.input.split(/[\\/]/).pop()}</td>
                    <td><span class={`badge ${routeClass(result.route)}`}>{result.route || 'Skip'}</span></td>
                    <td class="num">{formatBytes(result.inputSize)}</td>
                    <td class="num">{formatBytes(result.outputSize)}</td>
                    <td class:error={Boolean(result.error)} class:success={!result.error && !result.skipped}>{result.error || result.skipReason || (result.cancelled ? 'cancelled' : 'done')}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>jxlinfo</h3>
            <div class="in kv">
              <div><span>Decoded results</span><span>verified by djxl</span></div>
              <div><span>JPEG reconstruction</span><span class="success">checked on transcode</span></div>
              <div><span>Output balance</span><span>{formatBytes(summary?.bytesOut ?? 0)} written</span></div>
            </div>
          </div>
          <div class="banner info" style="margin:0"><span class="ic">i</span><span>Detailed jxlinfo metadata will appear when a result is selected.</span></div>
        </div>
      </div>
    </div>
  {:else if view === 'tools'}
    <div class="body">
      {#if toolchainError}
        <div class="banner warn"><span class="ic">!</span><span>{toolchainError}</span><button class="btn ghost" style="margin-left:auto" onclick={() => void refreshToolchain()}>Retry</button></div>
      {/if}
      {#if toolchain?.flagsLocked}
        <div class="banner warn"><span class="ic">!</span><span><b>Expert flags are locked.</b> Installed libjxl {toolchain.flagToolVersion || 'unknown'} differs from generated flags {toolchain.flagBaseVersion}.</span></div>
      {/if}
      <div class="cols">
        <div class="card">
          <h3>libjxl</h3>
          <div class="in">
            <div class="row"><span class="k">Installed</span><span class="v">{toolchain?.installedVersion || 'not installed'}</span></div>
            <div class="row"><span class="k">cjxl</span><span class="v">{toolchain?.cjxlVersion || '-'}</span></div>
            <div class="row"><span class="k">djxl</span><span class="v">{toolchain?.djxlVersion || '-'}</span></div>
            <div class="row"><span class="k">jxlinfo</span><span class="v">{toolchain?.jxlinfoVersion || '-'}</span></div>
            <div class="row"><span class="k">Latest version</span><span class:warning={toolchain?.updateAvailable} class="v">{toolchain?.latestVersion || '-'}</span></div>
            <div class="row"><span class="k">Asset</span><span class="v">jxl-x64-windows-static.zip</span></div>
            {#if toolchain?.updateAvailable || toolchain?.needsInstall}
              <button class="btn primary" style="margin-top:10px;background:var(--p-encode)" onclick={() => void installToolchain()} disabled={busy}>{toolchain?.needsInstall ? 'Install toolchain' : `Update to ${toolchain.latestVersion}`}</button>
            {/if}
          </div>
        </div>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Explorer context menu</h3>
            <div class="in">
              <div class="row"><span class="k">Status</span><span class:success={contextMenuRegistered} class:muted={!contextMenuRegistered} class="v">{contextMenuRegistered ? 'registered' : 'not registered'}</span></div>
              <div class="row"><span class="k">Preset</span><span class="v">{bindings.contextMenu || 'not bound'}</span></div>
              <div class="mini" style="margin-top:8px">Per-user registration; Windows 11 shows it under Show more options.</div>
              <div style="display:flex;gap:8px;margin-top:10px;flex-wrap:wrap">
                <button class="btn" onclick={() => void registerContextMenu()} disabled={!bindings.contextMenu}>Register</button>
                <button class="btn danger" onclick={() => void unregisterContextMenu()} disabled={!contextMenuRegistered}>Remove entry</button>
              </div>
            </div>
          </div>
          <div class="card">
            <h3>Flag changes</h3>
            <div class="in kv">
              <div><span>Base</span><span>{toolchain?.flagBaseVersion || '-'}</span></div>
              <div><span>Installed</span><span>{toolchain?.flagToolVersion || '-'}</span></div>
              <div><span>Added</span><span>{toolchain?.addedFlags?.join(', ') || 'none'}</span></div>
              <div><span>Removed</span><span>{toolchain?.removedFlags?.join(', ') || 'none'}</span></div>
            </div>
          </div>
          <div class="card">
            <h3>Storage locations</h3>
            <div class="in kv">
              <div><span>Settings</span><span>%APPDATA%\\jxleet\\config.yaml</span></div>
              <div><span>Presets</span><span>%APPDATA%\\jxleet\\presets\\</span></div>
              <div><span>Binaries</span><span>%LOCALAPPDATA%\\jxleet\\bin\\</span></div>
              <div><span>Logs</span><span>%LOCALAPPDATA%\\jxleet\\logs\\</span></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  {:else if view === 'presets'}
    <div class="body">
      <div class="cols wide-right">
        <div class="card">
          <h3>Preset library <span class="r">{presets.length} stored</span></h3>
          {#if presets.length === 0}
            <div class="empty">No presets yet. Create one or copy a YAML preset into %APPDATA%\\jxleet\\presets\\.</div>
          {:else}
            <table class="files" data-testid="preset-table">
              <thead><tr><th>Name</th><th>Description</th><th>Output</th></tr></thead>
              <tbody>
                {#each presets as preset}
                  <tr aria-selected={selectedPreset === preset.name} onclick={() => selectedPreset = preset.name}>
                    <td class="fn">{preset.name}</td><td>{preset.description || '-'}</td><td class="num">{preset.policy}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
          <div class="in" style="display:flex;gap:8px;border-top:1px solid var(--line-soft);flex-wrap:wrap">
            <button class="btn" data-testid="preset-new" onclick={() => void createPreset()}>New preset</button>
            <button class="btn ghost" onclick={() => void duplicatePreset()} disabled={!selectedPreset}>Duplicate</button>
            <button class="btn ghost" onclick={() => void renamePreset()} disabled={!selectedPreset}>Rename</button>
            <button class="btn danger" style="margin-left:auto" onclick={() => void deletePreset()} disabled={!selectedPreset}>Delete</button>
          </div>
        </div>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Which preset each entry point uses</h3>
            <div class="in" data-testid="preset-bindings">
              <div style="padding:2px 0 8px">
                <div style="font-size:12px;color:var(--ink);margin-bottom:3px">Graphical interface</div>
                <select style="width:100%" data-testid="bind-gui" value={bindings.gui} onchange={(event) => void setBinding('gui', (event.currentTarget as HTMLSelectElement).value)}>
                  <option value="">Select a preset</option>
                  {#each presets as preset}<option value={preset.name}>{preset.name}</option>{/each}
                </select>
                <div class="mini" style="margin-top:4px">Used by the main window.</div>
              </div>
              <div style="padding:8px 0;border-top:1px solid var(--line-soft)">
                <div style="font-size:12px;color:var(--ink);margin-bottom:3px">File-path invocation</div>
                <select style="width:100%" data-testid="bind-cli" value={bindings.cli} onchange={(event) => void setBinding('cli', (event.currentTarget as HTMLSelectElement).value)}>
                  <option value="">Select a preset</option>
                  {#each presets as preset}<option value={preset.name}>{preset.name}</option>{/each}
                </select>
                <div class="mini" style="margin-top:4px">Used by Lightroom and CLI invocations without --preset.</div>
              </div>
              <div style="padding:8px 0 2px;border-top:1px solid var(--line-soft)">
                <div style="font-size:12px;color:var(--ink);margin-bottom:3px">Explorer context menu</div>
                <select style="width:100%" data-testid="bind-menu" value={bindings.contextMenu} onchange={(event) => void setBinding('contextmenu', (event.currentTarget as HTMLSelectElement).value)}>
                  <option value="">Select a preset</option>
                  {#each presets as preset}<option value={preset.name}>{preset.name}</option>{/each}
                </select>
                <div class="mini" style="margin-top:4px">The menu text will carry this preset name.</div>
              </div>
            </div>
          </div>
          <div class="card">
            <h3>Invocation from outside</h3>
            <div class="in">
              <div class="cmd" style="margin:0">jxleet.exe --preset="Web d1.5 e7" "%1"</div>
              <div class="mini" style="margin-top:7px">An explicit --preset overrides the CLI binding. Unknown names fail rather than fall back.</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <div class="statusbar">
    {#if toolchain?.installedVersion}
      <span class="ok"><span class="dotled"></span>cjxl {toolchain.installedVersion}</span>
    {:else}
      <span class="warning"><span class="dotled"></span>libjxl not installed</span>
    {/if}
    <span class="spacer"></span>
    {#if view === 'running' || view === 'automatic'}
      <span>{progress.completed} done - {progress.failed} failed</span><span>{formatRate(progress.throughput)}</span>
    {:else if view === 'tools' && toolchain?.updateAvailable}
      <span class="up">Update available</span>
    {:else if files.length > 0}
      <span>{files.length} files - {formatBytes(totalSize)}</span>
    {:else}
      <span>Queue empty</span>
    {/if}
    <span>1 instance</span>
  </div>
</main>
