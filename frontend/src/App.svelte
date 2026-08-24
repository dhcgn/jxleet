<script lang="ts">
  import { onMount } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { Service } from '../bindings/github.com/dhcgn/jxleet/internal/app';
  import type {
    Bindings,
    CommandPreview,
    ConversionOptions,
    ConversionSummary,
    FilePreview,
    FileUpdate,
    FlagInfo,
    FlagOverride,
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

  const hiddenExpertFlags = new Set(['--verbose', '--help', '--version', '--quiet']);

  let view = $state<View>('drop');
  let mode = $state<'basic' | 'expert'>('basic');
  let routeMode = $state<RouteMode>('lossy');
  let qualityUnit = $state<QualityUnit>('distance');
  let presetName = $state('');
  let selectedPreset = $state('');
  let presets = $state<PresetSummary[]>([]);
  let flagDefinitions = $state<FlagInfo[]>([]);
  let expertOverrides = $state<FlagOverride[]>([]);
  let expertFlagsReset = $state(false);
  let bindings = $state<Bindings>({ gui: '', cli: '', contextMenu: '' });
  let inputPaths = $state<string[]>([]);
  let files = $state<FilePreview[]>([]);
  let results = $state<FileUpdate[]>([]);
  let selectedResultInput = $state('');
  let metadataOutput = $state('');
  let metadataError = $state('');
  let metadataLoading = $state(false);
  let metadataRequest = 0;
  let previewRequest = 0;
  let previewTimer: ReturnType<typeof setTimeout> | undefined;
  let commandPreviews = $state<CommandPreview[]>([]);
  let commandPreviewError = $state('');
  let commandPreviewRequest = 0;
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
  let presetPolicyDraft = $state('');
  let lossyDistance = $state(1.0);

  let routeCounts = $derived.by(() => ({
    Transcode: files.filter((file) => file.route === 'Transcode').length,
    Reencode: files.filter((file) => file.route === 'Reencode').length,
    Encode: files.filter((file) => file.route === 'Encode').length,
    Skip: files.filter((file) => file.skip).length,
  }));
  let totalSize = $derived(files.reduce((total, file) => total + file.size, 0));
  let selectedPresetData = $derived(presets.find((preset) => preset.name === selectedPreset) ?? null);
  let selectedPresetRules = $derived(selectedPresetData?.rules ?? []);
  let selectedPresetReadOnly = $derived(presets.find((preset) => preset.name === selectedPreset)?.readOnly ?? false);
  let presetPolicyDirty = $derived(Boolean(selectedPresetData) && presetPolicyDraft !== selectedPresetData?.policy);
  let dominantRoute = $derived(
    routeCounts.Reencode >= routeCounts.Encode && routeCounts.Reencode >= routeCounts.Transcode
      ? 'Reencode'
      : routeCounts.Encode >= routeCounts.Transcode
        ? 'Encode'
        : 'Transcode',
  );
  let quality = $derived(Math.round(qualityFromDistance(distance)));
  let outOfRange = $derived(
    routeMode !== 'lossless' &&
      (qualityUnit === 'distance' ? distance < 0.5 || distance > 3 : quality < 68 || quality > 96),
  );
  let canConvert = $derived(inputPaths.length > 0 && files.length > 0 && presetName !== '' && !busy);
  let selectedResult = $derived(results.find((result) => result.input === selectedResultInput) ?? null);
  let processing = $derived(busy && (view === 'basic' || view === 'expert'));
  let resultByInput = $derived.by(() => {
    const map = new Map<string, FileUpdate>();
    for (const result of results) map.set(result.input, result);
    return map;
  });
  let fileStatuses = $derived.by(() => {
    const map = new Map<string, string>();
    let runningLeft = processing ? progress.inFlight : 0;
    for (const file of files) {
      const result = resultByInput.get(file.path);
      if (result) {
        map.set(file.path, result.error ? 'failed' : result.skipped ? 'skipped' : result.cancelled ? 'cancelled' : 'done');
      } else if (processing && runningLeft > 0) {
        map.set(file.path, 'running');
        runningLeft -= 1;
      } else if (processing) {
        map.set(file.path, 'waiting');
      } else {
        map.set(file.path, '');
      }
    }
    return map;
  });
  let flagSections = $derived.by(() => {
    const sections: { name: string; flags: FlagInfo[] }[] = [];
    for (const flag of flagDefinitions) {
      if (hiddenExpertFlags.has(flag.key)) continue;
      const section = flag.section || 'Other options';
      let group = sections.find((item) => item.name === section);
      if (!group) {
        group = { name: section, flags: [] };
        sections.push(group);
      }
      group.flags.push(flag);
    }
    return sections;
  });
  let visibleFlagCount = $derived(flagSections.reduce((count, section) => count + section.flags.length, 0));

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
        busy = true;
        // GUI runs stay in the Basic/Expert view (queue shown inline). Only the
        // coalesced auto path uses the compact window, and a cold CLI start with
        // no UI context falls back to the running screen.
        if (progress.coalesced > 1) {
          view = 'automatic';
        } else if (view === 'drop') {
          view = 'running';
        }
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
      if (summary?.cancelled) {
        errorMessage = '! cancelled by user';
      }
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
      flagDefinitions = (await Service.ListCJXLFlags()) ?? [];
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
      void refreshCommandPreview();
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
      expertFlags: expertOverrides,
      resetExpert: expertFlagsReset,
    };
  }

  async function refreshPreview(): Promise<FilePreview[] | null> {
    const request = ++previewRequest;
    if (inputPaths.length === 0) {
      files = [];
      void refreshCommandPreview();
      return [];
    }
    const paths = [...inputPaths];
    const options = currentOptions();
    try {
      const preview = (await Service.PreviewPaths(paths, options)) ?? [];
      if (request !== previewRequest) return null;
      files = preview;
      errorMessage = '';
      void refreshCommandPreview();
      return preview;
    } catch (error) {
      if (request !== previewRequest) return null;
      errorMessage = errorText(error);
      return null;
    }
  }

  function schedulePreviewRefresh(): void {
    clearTimeout(previewTimer);
    previewTimer = setTimeout(() => void refreshPreview(), 180);
  }

  async function refreshCommandPreview(): Promise<void> {
    const request = ++commandPreviewRequest;
    if (presetName === '') {
      commandPreviews = [];
      commandPreviewError = '';
      return;
    }
    try {
      const previews = (await Service.PreviewCommands(currentOptions())) ?? [];
      if (request !== commandPreviewRequest) return;
      commandPreviews = previews;
      commandPreviewError = '';
    } catch (error) {
      if (request !== commandPreviewRequest) return;
      commandPreviews = [];
      commandPreviewError = errorText(error);
    }
  }

  function acceptPaths(paths: string[]): void {
    const incoming = paths.filter((path) => path.trim() !== '');
    if (incoming.length === 0) return;
    const wasRunning = view === 'running' || view === 'automatic';
    if (view === 'done') {
      results = [];
      summary = null;
      selectedResultInput = '';
      metadataOutput = '';
      metadataError = '';
      metadataLoading = false;
      metadataRequest += 1;
    }
    const next = [...inputPaths, ...incoming];
    inputPaths = [...new Set(next)];
    errorMessage = '';
    view = wasRunning ? 'automatic' : 'basic';
    void refreshPreview();
  }

  async function openFile(): Promise<void> {
    try {
      const paths = (await Service.OpenFiles()) ?? [];
      acceptPaths(paths);
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function openFolder(): Promise<void> {
    try {
      const paths = (await Service.OpenFolders()) ?? [];
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
      errorMessage = 'The WebView did not expose local paths. Use Open File or Open Folder.';
      return;
    }
    acceptPaths(paths);
  }

  async function startConversion(): Promise<void> {
    if (inputPaths.length === 0) {
      errorMessage = 'Drop files or use Open File/Open Folder first.';
      return;
    }
    if (presetName === '') {
      errorMessage = 'Select a preset in the toolbar before converting.';
      return;
    }
    if (outputPolicy === 'replace') {
      const preview = await refreshPreview();
      if (preview === null) return;
      const irreversible = preview.filter((file) => file.route === 'Reencode' || (file.route === 'Encode' && distance > 0)).length;
      if (irreversible > 0 && !window.confirm(`Replace the originals for ${irreversible} irreversible file${irreversible === 1 ? '' : 's'}? They will be sent to the recycle bin after verification.`)) {
        return;
      }
    }
    busy = true;
    errorMessage = '';
    results = [];
    selectedResultInput = '';
    metadataOutput = '';
    metadataError = '';
    metadataLoading = false;
    metadataRequest += 1;
    summary = null;
    progress = { ...progress, total: files.length, completed: 0, failed: 0, skipped: 0, inFlight: 0, percent: 0, paused: false };
    view = 'basic';
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
      await refreshPreview();
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
      selectPreset(name.trim());
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
      selectPreset(name.trim());
      await refreshPreview();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function deletePreset(): Promise<void> {
    if (!selectedPreset || selectedPresetReadOnly || !window.confirm(`Delete "${selectedPreset}"?`)) return;
    try {
      await Service.DeletePreset(selectedPreset);
      if (presetName === selectedPreset) presetName = '';
      selectedPreset = '';
      presetPolicyDraft = '';
      await refreshPresets();
      await refreshPreview();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function refreshPresets(): Promise<void> {
    presets = (await Service.ListPresets()) ?? [];
  }

  function selectPreset(name: string): void {
    selectedPreset = name;
    presetPolicyDraft = presets.find((preset) => preset.name === name)?.policy ?? '';
  }

  async function savePresetPolicy(): Promise<void> {
    if (!selectedPreset || selectedPresetReadOnly || !presetPolicyDirty) return;
    try {
      await Service.SavePresetOutputPolicy(selectedPreset, presetPolicyDraft);
      await refreshPresets();
      selectPreset(selectedPreset);
    } catch (error) {
      errorMessage = errorText(error);
    }
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

  async function openStorage(location: string): Promise<void> {
    try {
      await Service.OpenStorageLocation(location);
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
    previewRequest += 1;
    commandPreviews = [];
    commandPreviewError = '';
    commandPreviewRequest += 1;
    results = [];
    selectedResultInput = '';
    metadataOutput = '';
    metadataError = '';
    metadataLoading = false;
    metadataRequest += 1;
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

  function compactFileName(name: string): string {
    return name.length > 9 ? `${name.slice(0, 3)}...${name.slice(-6)}` : name;
  }

  function compactPath(path: string, maxLength = 80): string {
    if (path.length <= maxLength) return path;
    const tail = Math.max(16, Math.floor(maxLength * 0.35));
    const head = Math.max(3, maxLength - tail - 3);
    return `${path.slice(0, head)}...${path.slice(-tail)}`;
  }

  function fileStatus(file: FilePreview): string {
    return fileStatuses.get(file.path) ?? '';
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
    distance = Number((event.currentTarget as HTMLInputElement).value);
    if (routeMode === 'lossy') lossyDistance = distance;
    void refreshCommandPreview();
    schedulePreviewRefresh();
  }

  function setQuality(event: Event): void {
    const position = Number((event.currentTarget as HTMLInputElement).value);
    const value = 100 - position; // slider is inverted so best quality (100) sits on the left, matching distance 0
    distance = value >= 100 ? 0 : 0.1 + (100 - value) * 0.09;
    if (routeMode === 'lossy') lossyDistance = distance;
    void refreshCommandPreview();
    schedulePreviewRefresh();
  }

  function setEffort(event: Event): void {
    effort = Number((event.currentTarget as HTMLInputElement).value);
    void refreshCommandPreview();
    schedulePreviewRefresh();
  }

  function expertFlagValue(key: string): string {
    return expertOverrides.find((override) => override.key === key)?.value ?? '';
  }

  function expertFlagEnabled(key: string): boolean {
    return expertOverrides.some((override) => override.key === key && override.valueless);
  }

  function setExpertFlagValue(key: string, value: string): void {
    const next = expertOverrides.filter((override) => override.key !== key);
    if (value.trim() !== '') {
      next.push({ key, value, valueless: false });
    }
    expertOverrides = next;
    void refreshCommandPreview();
  }

  function setExpertFlagEnabled(key: string, enabled: boolean): void {
    const next = expertOverrides.filter((override) => override.key !== key);
    if (enabled) {
      next.push({ key, value: '', valueless: true });
    }
    expertOverrides = next;
    void refreshCommandPreview();
  }

  function isLinkedFlagKey(key: string): boolean {
    return ['--distance', '--quality', '--effort', '--lossless_jpeg', '--num_threads'].includes(key);
  }

  function isLinkedFlag(flag: FlagInfo): boolean {
    return isLinkedFlagKey(flag.key);
  }

  function linkedFlagValue(flag: FlagInfo): string {
    switch (flag.key) {
      case '--distance':
        return distance.toFixed(1);
      case '--quality':
        return quality.toString();
      case '--effort':
        return effort.toString();
      case '--lossless_jpeg':
        return jpegLossless ? '1' : '0';
      case '--num_threads':
        return threads.toString();
      default:
        return '';
    }
  }

  function flagLabel(flag: FlagInfo): string {
    const short = flag.short ? `-${flag.short}` : '';
    const long = flag.long ? `--${flag.long}` : '';
    return short && long ? `${short}, ${long}` : short || long;
  }

  function resetExpertFlags(): void {
    expertOverrides = [];
    expertFlagsReset = true;
    void refreshCommandPreview();
  }

  function setRouteMode(next: RouteMode): void {
    if (next === routeMode) return;
    if (next === 'lossless') {
      lossyDistance = distance > 0 ? distance : lossyDistance;
      distance = 0;
    } else {
      distance = lossyDistance > 0 ? lossyDistance : 1.0;
    }
    routeMode = next;
    void refreshCommandPreview();
  }

  function qualityStatusText(): string {
    if (routeMode === 'lossless') return 'Lossless: distance fixed at 0';
    return outOfRange ? 'Out of recommended range (too high or too low)' : 'Inside recommended range';
  }

  async function selectResult(result: FileUpdate): Promise<void> {
    selectedResultInput = result.input;
    metadataOutput = '';
    metadataError = '';
    metadataLoading = true;
    const request = ++metadataRequest;
    if (result.error || result.skipped || result.cancelled || !result.output) {
      metadataError = result.error || result.skipReason || (result.cancelled ? 'Conversion was cancelled.' : 'No JXL output was produced for this result.');
      metadataLoading = false;
      return;
    }
    try {
      const output = await Service.InspectJXL(result.output);
      if (request !== metadataRequest) return;
      metadataOutput = output;
    } catch (error) {
      if (request !== metadataRequest) return;
      metadataError = errorText(error);
    } finally {
      if (request === metadataRequest) metadataLoading = false;
    }
  }

  function errorText(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  function dismissMessage(): void {
    errorMessage = '';
  }
</script>

<svelte:head>
  <title>jxleet</title>
</svelte:head>

<svelte:window onkeydown={(event) => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'o') {
    event.preventDefault();
    void openFile();
  }
}} />

<main class:running={view === 'running' || view === 'automatic'} class:narrow={view === 'automatic'} class="app">
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
    <div class="intake-actions" aria-label="Add inputs">
      <button class="btn" onclick={() => void openFile()}>Open File</button>
      <button class="btn" onclick={() => void openFolder()}>Open Folder</button>
    </div>
    <button class="btn ghost" onclick={() => { view = 'presets'; mode = 'basic'; }}>Presets</button>
    <button class="btn ghost" onclick={() => { view = 'tools'; mode = 'basic'; }}>Tools</button>
    <span class="spacer"></span>
    {#if view === 'done'}
      <button class="btn" onclick={resetDrop}>New drop</button>
    {:else if view === 'presets' || view === 'tools'}
      <button class="btn" onclick={() => { view = inputPaths.length > 0 ? 'basic' : 'drop'; }}>Back</button>
    {/if}
  </div>

  {#if errorMessage}
    <div class="banner warn" role="alert">
      <span class="ic">!</span><span>{errorMessage}</span>
      <button class="alert-close" aria-label="Dismiss message" title="Dismiss" onclick={dismissMessage}>x</button>
    </div>
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
        data-file-drop-target="dropzone"
        role="button"
        tabindex="0"
        aria-label="Drop files or folders"
        ondragover={(event) => { event.preventDefault(); dragging = true; }}
        ondragleave={() => { dragging = false; }}
        ondrop={handleDrop}
        onclick={() => void openFile()}
        onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') void openFile(); }}
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
          <button class="btn primary" style="background:var(--p-encode)" onclick={(event) => { event.stopPropagation(); void openFile(); }}>Open File</button>
          <button class="btn" onclick={(event) => { event.stopPropagation(); void openFolder(); }}>Open Folder</button>
        </div>
      </div>
    </div>
  {:else if view === 'basic'}
    <div class="body">
      {#if processing}
        <div class="run-strip" data-testid="run-strip">
          <div class="run-head">
            <span class="badge b-reencode">{progress.paused ? 'Paused' : 'Converting'}</span>
            <span class="run-count">{progress.completed + progress.failed + progress.skipped} of {progress.total}</span>
            <span class="mini">{formatEta(progress.etaSeconds)} remaining - {formatRate(progress.throughput)}</span>
            <span class="spacer"></span>
            <button class="btn" onclick={() => void togglePause()}>{progress.paused ? 'Resume' : 'Pause'}</button>
            <button class="btn danger" data-testid="cancel" onclick={() => void cancelConversion()}>Cancel</button>
          </div>
          <div class="bar"><i style={`width:${Math.min(100, progress.percent)}%`}></i></div>
        </div>
      {/if}
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
          {#if inputPaths.length === 0}
            <div class="empty">No files selected. Use Open File or Open Folder above to add inputs.</div>
          {:else}
            {#if presetName === ''}
              <div class="banner info" style="margin:10px 10px 0"><span class="ic">i</span><span>Files are selected. Select a preset in the toolbar to classify their routes.</span></div>
            {/if}
            {#if files.length === 0}
              <div class="empty">The selected paths could not be previewed.</div>
            {:else}
            <table class="files basic-files" data-testid="file-table">
              <colgroup>
                <col class="bf-file-col" />
                <col class="bf-route-col" />
                <col class="bf-settings-col" />
                <col class="bf-size-col" />
                {#if processing || results.length > 0}<col class="bf-status-col" />{/if}
              </colgroup>
              <thead><tr><th>File</th><th>Route</th><th>Settings</th><th style="text-align:right">Size</th>{#if processing || results.length > 0}<th>Status</th>{/if}</tr></thead>
              <tbody>
                {#each files as file}
                  {@const status = fileStatus(file)}
                  <tr>
                    <td class="fn" title={file.path}>{compactFileName(file.name)}</td>
                    <td class="route-cell"><span class={`badge ${routeClass(file.route)}`}>{file.skip ? 'skip' : file.route || 'pending'}</span></td>
                    <td class="settings-cell" title={file.flagsSet ? `${file.settings} + extra flags` : file.settings}>
                      {#if file.skip}<span class="mini">{file.reason || '-'}</span>{:else}<span class="mono-mini">{file.settings || '-'}</span>{#if file.flagsSet}<span class="flag-chip" title="Extra cjxl flags applied">+flags</span>{/if}{/if}
                    </td>
                    <td class="num">{formatBytes(file.size)}</td>
                    {#if processing || results.length > 0}<td class="status-cell" class:success={status === 'done'} class:error={status === 'failed'}>{status}</td>{/if}
                  </tr>
                {/each}
              </tbody>
            </table>
            {/if}
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
                <span class="qnum" class:out-of-range={outOfRange}>{qualityUnit === 'distance' ? distance.toFixed(1) : quality}</span>
                <span class="mini range-status" class:out-of-range={outOfRange} style="margin-left:auto">{qualityStatusText()}</span>
              </div>
              {#if qualityUnit === 'distance'}
                <div class="quality-range" class:out-of-range={outOfRange} style="--zone-a:2%;--zone-b:4%;--zone-c:8%;--zone-d:12%">
                  <input type="range" min="0" max="25" step="0.1" value={distance} oninput={setDistance} aria-label="Distance" />
                </div>
                <div class="quality-guidance"><span>Recommended: 0.5 .. 3.0</span><span>1.0 = visually lossless</span></div>
              {:else}
                <div class="quality-range" class:out-of-range={outOfRange} style="--zone-a:4%;--zone-b:14%;--zone-c:22%;--zone-d:32%">
                  <input type="range" min="0" max="100" step="1" value={100 - quality} oninput={(event) => setQuality(event)} aria-label="Quality" />
                </div>
                <div class="quality-guidance"><span>Recommended: 68 .. 96</span><span>90 = visually lossless</span></div>
              {/if}
              <div class="effort-basic" style="border-top:1px solid var(--line-soft);margin-top:8px;padding-top:8px">
                <div style="display:flex;align-items:baseline;gap:8px">
                  <span class="k">Effort</span>
                  <span class="v">{effort} - {effortNames[effort - 1]}</span>
                  <span class="mini" style="margin-left:auto">{effort === 7 ? 'default' : effort >= 9 ? 'slow' : effort <= 3 ? 'fast' : ''}</span>
                </div>
                <input type="range" min="1" max="10" step="1" value={effort} oninput={setEffort} data-testid="effort-range-basic" aria-label="Effort" />
                <div class="quality-guidance"><span>1 = fastest</span><span>10 = smallest</span></div>
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

          <button class="btn primary convert-action" style={`background:var(--p-${dominantRoute === 'Transcode' ? 'transcode' : dominantRoute === 'Encode' ? 'encode' : 'reencode'});padding:11px`} data-testid="start-convert" onclick={() => void startConversion()} disabled={!canConvert}>
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
            <button aria-pressed={routeMode === 'lossy'} onclick={() => setRouteMode('lossy')}>Lossy</button>
            <button aria-pressed={routeMode === 'lossless'} onclick={() => setRouteMode('lossless')}>Lossless</button>
          </div>
        </div>
        <span class="spacer"></span>
        <button class="btn ghost" onclick={() => { setRouteMode('lossy'); distance = 1; effort = 7; jpegLossless = true; }}>Reset to preset</button>
      </div>
      <div class="cols wide-right">
        <div class="card">
          <h3>Effort <span class="r">what this level adds</span></h3>
          <div class="in">
            <div class="ladder-head">
              <span class="nm">{effortNames[effort - 1]}</span>
              <span class="hint">{effort === 7 ? 'cjxl default' : effort >= 9 ? 'noticeably slower' : effort <= 3 ? 'very fast, larger file' : ''}</span>
            </div>
            <div class="effort-slider">
              <input type="range" min="1" max="10" value={effort} oninput={setEffort} data-testid="effort-range" aria-label="Effort" />
            </div>
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
                    <td class="mode">
                      <span class="capability-icon" class:capability-yes={tool.lossy} role="img" aria-label={tool.lossy ? 'Lossy mode supported' : 'Lossy mode not supported'} title={tool.lossy ? 'Lossy mode supported' : 'Lossy mode not supported'}>
                        {#if tool.lossy}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8 3 3 7-7"></path></svg>{:else}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4 4 12"></path></svg>{/if}
                      </span>
                    </td>
                    <td class="mode">
                      <span class="capability-icon" class:capability-yes={tool.lossless} role="img" aria-label={tool.lossless ? 'Lossless mode supported' : 'Lossless mode not supported'} title={tool.lossless ? 'Lossless mode supported' : 'Lossless mode not supported'}>
                        {#if tool.lossless}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8 3 3 7-7"></path></svg>{:else}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4 4 12"></path></svg>{/if}
                      </span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
            {#if commandPreviewError}
              <div class="banner warn" data-testid="cmd-preview-error"><span class="ic">!</span><span>{commandPreviewError}</span></div>
            {:else if commandPreviews.length === 0}
              <div class="cmd" data-testid="cmd-preview">Select a preset to preview the resolved cjxl command.</div>
            {:else}
              <div class="command-previews" data-testid="cmd-preview">
                {#each commandPreviews as preview}
                  <div class="command-preview">
                    <div class="mini">{preview.matches?.join(', ') ?? 'default rule'}</div>
                    <div class="cmd">{preview.command}</div>
                  </div>
                {/each}
              </div>
            {/if}
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
                <span class="qnum" class:out-of-range={outOfRange}>{qualityUnit === 'distance' ? distance.toFixed(1) : quality}</span>
                <span class="mini range-status" class:out-of-range={outOfRange}>{qualityStatusText()}</span>
              </div>
              {#if routeMode === 'lossless'}
                <div class="quality-range locked">
                  <input type="range" min="0" max="25" step="0.1" value="0" disabled aria-label="Distance fixed at zero in lossless mode" />
                </div>
                <div class="quality-guidance"><span>Lossless: distance is fixed at 0</span></div>
              {:else if qualityUnit === 'distance'}
                <div class="quality-range" class:out-of-range={outOfRange} style="--zone-a:2%;--zone-b:4%;--zone-c:8%;--zone-d:12%">
                  <input type="range" min="0" max="25" step="0.1" value={distance} oninput={setDistance} aria-label="Distance" />
                </div>
                <div class="quality-guidance"><span>Recommended: 0.5 .. 3.0</span><span>1.0 = visually lossless</span></div>
              {:else}
                <div class="quality-range" class:out-of-range={outOfRange} style="--zone-a:4%;--zone-b:14%;--zone-c:22%;--zone-d:32%">
                  <input type="range" min="0" max="100" step="1" value={100 - quality} oninput={setQuality} aria-label="Quality" />
                </div>
                <div class="quality-guidance"><span>Recommended: 68 .. 96</span><span>90 = visually lossless</span></div>
              {/if}
              <div class="banner info" style="margin:10px 0 0"><span class="ic">i</span><span>Distance and quality control the same quantity. The marker shows the visually-lossless reference point.</span></div>
            </div>
          </div>
          <div class="card">
            <h3>Further flags <span class="r">{visibleFlagCount} cjxl flags</span></h3>
            <div class="in">
              <div class="banner info flag-notice">
                <span class="ic">i</span>
                <span>Changes apply to the next conversion only. Use Convert to test the current cjxl command; they are not saved automatically.</span>
              </div>
              {#if toolchain?.flagsLocked}
                <div class="banner warn"><span class="ic">!</span><span>Expert flags are locked because the installed cjxl version differs from the generated help.</span></div>
              {/if}
              <div class="flag-actions">
                <button class="btn ghost" onclick={resetExpertFlags} disabled={Boolean(toolchain?.flagsLocked)}>Reset Expert flags</button>
                <span class="mini">{expertFlagsReset ? 'Expert flags reset: cjxl defaults apply.' : 'Reset clears explicit flags so cjxl defaults apply.'}</span>
              </div>
              {#each flagSections as section}
                <div class="flag-section">
                  <div class="eyebrow">{section.name}</div>
                  {#each section.flags as flag}
                    {@const linked = isLinkedFlag(flag)}
                    <div class="flag-row" title={flag.description}>
                      <div class="flag-copy">
                        <span class="flag-name">{flagLabel(flag)}</span>
                        {#if flag.valueSpec}<span class="flag-spec">{flag.valueSpec}</span>{/if}
                        <span class="flag-description">{flag.description || 'No help text available.'}</span>
                      </div>
                      {#if flag.takesValue}
                        <input
                          class="flag-value"
                          type="text"
                          value={linked ? linkedFlagValue(flag) : expertFlagValue(flag.key)}
                          placeholder={linked ? '' : 'cjxl default'}
                          aria-label={flagLabel(flag)}
                          title={flag.description}
                          disabled={linked || Boolean(toolchain?.flagsLocked)}
                          oninput={(event) => setExpertFlagValue(flag.key, (event.currentTarget as HTMLInputElement).value)}
                        />
                      {:else}
                        <input
                          class="flag-toggle"
                          type="checkbox"
                          checked={expertFlagEnabled(flag.key)}
                          aria-label={flagLabel(flag)}
                          title={flag.description}
                          disabled={Boolean(toolchain?.flagsLocked)}
                          onchange={(event) => setExpertFlagEnabled(flag.key, (event.currentTarget as HTMLInputElement).checked)}
                        />
                      {/if}
                    </div>
                  {/each}
                </div>
              {/each}
            </div>
          </div>
          <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" onclick={() => void startConversion()} disabled={!canConvert}>Convert {files.length} files</button>
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
            <table class="files results-table" data-testid="result-table">
              <colgroup>
                <col class="result-file-col" />
                <col class="result-route-col" />
                <col class="result-before-col" />
                <col class="result-after-col" />
                <col class="result-status-col" />
              </colgroup>
              <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Before</th><th style="text-align:right">After</th><th>Status</th></tr></thead>
              <tbody>
                {#each results as result}
                  <tr class:selected={selectedResultInput === result.input} aria-selected={selectedResultInput === result.input} tabindex="0" onclick={() => void selectResult(result)} onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') void selectResult(result); }}>
                    <td class="fn" title={result.input}>{compactPath(result.input)}</td>
                    <td class="route-cell"><span class={`badge ${routeClass(result.route)}`}>{result.route || 'Skip'}</span></td>
                    <td class="num">{formatBytes(result.inputSize)}</td>
                    <td class="num">{formatBytes(result.outputSize)}</td>
                    <td class="status-cell" title={result.error || result.skipReason || (result.cancelled ? 'cancelled' : 'done')} class:error={Boolean(result.error)} class:success={!result.error && !result.skipped}>{result.error || result.skipReason || (result.cancelled ? 'cancelled' : 'done')}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>jxlinfo <span class="r">{selectedResult ? (selectedResult.output ? compactPath(selectedResult.output, 34) : 'unavailable') : 'select a result'}</span></h3>
            <div class="in">
              {#if !selectedResult}
                <div class="empty">Select a result row to inspect its detailed JPEG XL metadata.</div>
              {:else if metadataLoading}
                <div class="empty">Reading jxlinfo metadata...</div>
              {:else if metadataError}
                <div class="banner warn" style="margin:0"><span class="ic">!</span><span>{metadataError}</span></div>
              {:else}
                <pre class="metadata-output">{metadataOutput}</pre>
              {/if}
            </div>
          </div>
          <div class="card">
            <h3>Output summary</h3>
            <div class="in kv">
              <div><span>Decoded results</span><span>verified by djxl</span></div>
              <div><span>JPEG reconstruction</span><span class="success">checked on transcode</span></div>
              <div><span>Output balance</span><span>{formatBytes(summary?.bytesOut ?? 0)} written</span></div>
            </div>
          </div>
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
            <div style="display:flex;gap:8px;margin-top:10px;flex-wrap:wrap">
              <button class="btn" data-testid="check-updates" onclick={() => void refreshToolchain()} disabled={busy}>Check for updates</button>
              {#if toolchain?.updateAvailable || toolchain?.needsInstall}
                <button class="btn primary" style="background:var(--p-encode)" onclick={() => void installToolchain()} disabled={busy}>{toolchain?.needsInstall ? 'Install toolchain' : `Update to ${toolchain.latestVersion}`}</button>
              {/if}
            </div>
            {#if toolchain && !toolchain.updateAvailable && !toolchain.needsInstall}
              <div class="mini" style="margin-top:6px">libjxl is up to date.</div>
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
              <div><span>Settings</span><span>%APPDATA%\\jxleet\\config.yaml</span><button class="icon-btn" aria-label="Open settings folder" title="Open in Explorer" onclick={() => void openStorage('config')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
              <div><span>Presets</span><span>%APPDATA%\\jxleet\\presets\\</span><button class="icon-btn" aria-label="Open presets folder" title="Open in Explorer" onclick={() => void openStorage('presets')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
              <div><span>Binaries</span><span>%LOCALAPPDATA%\\jxleet\\bin\\</span><button class="icon-btn" aria-label="Open binaries folder" title="Open in Explorer" onclick={() => void openStorage('bin')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
              <div><span>Logs</span><span>%LOCALAPPDATA%\\jxleet\\logs\\</span><button class="icon-btn" aria-label="Open logs folder" title="Open in Explorer" onclick={() => void openStorage('logs')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  {:else if view === 'presets'}
    <div class="body">
      <div class="cols wide-right">
        <div class="card">
          <h3>Preset library <span class="r">{presets.length} stored
            <button class="icon-btn" aria-label="Reload presets from folder" title="Reload from folder" data-testid="preset-refresh" onclick={() => void refreshPresets()}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8 8 0 1 0-.9 4"></path><path d="M20 4v6h-6"></path></svg></button>
            <button class="icon-btn" aria-label="Open preset storage folder" title="Open in Explorer" onclick={() => void openStorage('presets')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></span></h3>
          <div class="banner info" style="margin:10px 10px 0"><span class="ic">i</span><span>Rules are edited by changing the YAML files directly. Open the folder, edit a preset, then Reload. A JSON schema (<code>preset.schema.json</code>) is provided for editor validation.</span></div>
          {#if presets.length === 0}
            <div class="empty">No presets yet. Create one or copy a YAML preset into %APPDATA%\\jxleet\\presets\\.</div>
          {:else}
            <table class="files" data-testid="preset-table">
              <colgroup>
                <col class="preset-name-col" />
                <col class="preset-core-col" />
                <col class="preset-effort-col" />
                <col class="preset-jpeg-col" />
                <col class="preset-output-col" />
              </colgroup>
              <thead><tr><th>Preset</th><th>Core</th><th>Effort</th><th>JPEG</th><th>Output</th></tr></thead>
              <tbody>
                {#each presets as preset}
                  <tr class:selected={selectedPreset === preset.name} aria-selected={selectedPreset === preset.name} onclick={() => selectPreset(preset.name)}>
                    <td class="fn" title={preset.description}>{preset.name}{preset.readOnly ? ' (read-only)' : ''}</td>
                    <td class="num">{preset.coreValue}</td>
                    <td class="num">{preset.effort}</td>
                    <td class="num">{preset.jpegMode}</td>
                    <td class="num">{preset.policy}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
          <div class="in" style="display:flex;gap:8px;border-top:1px solid var(--line-soft);flex-wrap:wrap">
            <button class="btn" data-testid="preset-new" onclick={() => void createPreset()}>New preset</button>
            <button class="btn ghost" onclick={() => void duplicatePreset()} disabled={!selectedPreset}>Duplicate</button>
            <button class="btn ghost" onclick={() => void renamePreset()} disabled={!selectedPreset || selectedPresetReadOnly}>Rename</button>
            <button class="btn danger" style="margin-left:auto" onclick={() => void deletePreset()} disabled={!selectedPreset || selectedPresetReadOnly}>Delete</button>
          </div>
          {#if selectedPresetData}
            <div class="preset-details" data-testid="preset-details">
              <h3>{selectedPresetData.name} <span class="r">{selectedPresetReadOnly ? 'read-only' : 'details'}</span></h3>
              <div class="in">
                <div class="preset-policy-editor">
                  <label for="preset-output-policy">Output policy</label>
                  <div class="preset-policy-controls">
                    <select id="preset-output-policy" bind:value={presetPolicyDraft} disabled={selectedPresetReadOnly}>
                      <option value="alongside">Alongside</option>
                      <option value="subfolder">Into subfolder</option>
                      <option value="replace">Replace via recycle bin</option>
                    </select>
                    <button class="btn" onclick={() => void savePresetPolicy()} disabled={selectedPresetReadOnly || !presetPolicyDirty}>Save</button>
                  </div>
                  {#if selectedPresetReadOnly}
                    <div class="mini">This built-in preset is read-only. Duplicate it to change the policy.</div>
                  {/if}
                </div>
                <div class="rule-details">
                  <div class="eyebrow">File rules</div>
                  {#if selectedPresetRules.length === 0}
                    <div class="empty">No rules.</div>
                  {:else}
                    <table class="rule-table">
                      <thead><tr><th>File</th><th>Core</th><th>Effort</th><th>JPEG</th></tr></thead>
                      <tbody>
                        {#each selectedPresetRules as rule}
                          <tr>
                            <td class="rule-matches">{rule.matches?.join(', ') ?? ''}</td>
                            <td>{rule.coreValue}</td>
                            <td>{rule.effort}</td>
                            <td>{rule.jpegMode}</td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  {/if}
                </div>
              </div>
            </div>
          {/if}
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
