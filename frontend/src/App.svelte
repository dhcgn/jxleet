<script lang="ts">
  import { onMount } from 'svelte';
  import { Events } from '@wailsio/runtime';
  import { Service } from '../bindings/github.com/dhcgn/jxleet/internal/app';
  import type {
    Bindings,
    CollisionPrompt,
    CommandPreview,
    ConversionOptions,
    ConversionSummary,
    FilePreview,
    FileUpdate,
    FlagInfo,
    FlagOverride,
    HistoryEntry,
    PresetSummary,
    ProgressUpdate,
    Status,
    ToolchainProgress,
    ToolchainStatus,
    Update,
  } from '../bindings/github.com/dhcgn/jxleet/internal/app';
  import type { RouteMode, View } from './lib/types';
  import { distanceFromQuality, qualityFromDistance } from './lib/quality';
  import { formatBytes, formatRate } from './lib/format';
  import { sameFlags } from './lib/flags';
  import AutomaticView from './views/AutomaticView.svelte';
  import ToolsView from './views/ToolsView.svelte';
  import HistoryView from './views/HistoryView.svelte';
  import PresetsView from './views/PresetsView.svelte';
  import ExpertView from './views/ExpertView.svelte';
  import MainView from './views/MainView.svelte';


  let view = $state<View>('main');
  let routeMode = $state<RouteMode>('lossy');
  let presetName = $state('');
  let selectedPreset = $state('');
  let presets = $state<PresetSummary[]>([]);
  let flagDefinitions = $state<FlagInfo[]>([]);
  let expertOverrides = $state<FlagOverride[]>([]);
  let bindings = $state<Bindings>({ gui: '', cli: '', contextMenu: '' });
  let inputPaths = $state<string[]>([]);
  let files = $state<FilePreview[]>([]);
  let results = $state<FileUpdate[]>([]);

  // Grouped state: each object is one prop for a view component.
  let meta = $state({ selection: '', output: '', error: '', loading: false });
  let cmdPreview = $state<{ previews: CommandPreview[]; error: string }>({ previews: [], error: '' });
  let run = $state<{ busy: boolean; summary: ConversionSummary | null }>({ busy: false, summary: null });
  let tools = $state<{
    status: ToolchainStatus | null;
    error: string;
    installing: boolean;
    progress: ToolchainProgress | null;
    contextMenu: boolean;
  }>({ status: null, error: '', installing: false, progress: null, contextMenu: false });
  let history = $state<{
    entries: HistoryEntry[];
    loaded: boolean;
    meta: { entry: HistoryEntry | null; output: string; error: string; loading: boolean };
  }>({ entries: [], loaded: false, meta: { entry: null, output: '', error: '', loading: false } });
  let settings = $state({
    distance: 1.0,
    effort: 7,
    jpegLossless: true,
    outputPolicy: 'alongside',
    processes: 2,
    threads: 8,
    lossyDistance: 1.0,
  });

  // Request counters and timers are not reactive state.
  let metadataRequest = 0;
  let previewRequest = 0;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let commandPreviewRequest = 0;
  let historyMetaRequest = 0;

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
  let appUpdate = $state<Update | null>(null);
  let errorMessage = $state('');
  let loaded = $state(false);
  let sessionStats = $state({ count: 0, saved: 0 });
  let collisionPrompt = $state<CollisionPrompt | null>(null);
  let presetPolicyDraft = $state('');
  let presetCollisionDraft = $state('skip');

  let totalSize = $derived(files.reduce((total, file) => total + file.size, 0));
  let selectedPresetData = $derived(presets.find((preset) => preset.name === selectedPreset) ?? null);
  let selectedPresetReadOnly = $derived(presets.find((preset) => preset.name === selectedPreset)?.readOnly ?? false);
  let presetOutputDirty = $derived(
    Boolean(selectedPresetData) &&
      (presetPolicyDraft !== selectedPresetData?.policy || presetCollisionDraft !== selectedPresetData?.collision),
  );
  let quality = $derived(Math.round(qualityFromDistance(settings.distance)));
  let outOfRange = $derived(routeMode !== 'lossless' && (settings.distance < 0.5 || settings.distance > 3));
  let canConvert = $derived(inputPaths.length > 0 && files.length > 0 && presetName !== '' && !run.busy && !tools.installing);
  let resultByInput = $derived.by(() => {
    const map = new Map<string, FileUpdate>();
    for (const result of results) map.set(result.input, result);
    return map;
  });
  // Files that still need conversion: no result yet, or failed/cancelled (retry).
  // Successfully converted files are excluded so adding files after a finished
  // run only converts the new ones.
  let pendingPaths = $derived(
    files
      .filter((file) => {
        const result = resultByInput.get(file.path);
        return !result || result.error !== '' || result.cancelled;
      })
      .map((file) => file.path),
  );
  let activePresetSummary = $derived(presets.find((preset) => preset.name === presetName) ?? null);

  let presetChanged = $derived.by(() => {
    if (!coreSnapshot || presetName === '') return false;
    const snapshot: CoreSnapshot = coreSnapshot;
    return (
      settings.distance !== snapshot.distance ||
      settings.effort !== snapshot.effort ||
      settings.jpegLossless !== snapshot.jpegLossless ||
      settings.outputPolicy !== snapshot.policy ||
      !sameFlags(expertOverrides, snapshot.flags)
    );
  });
  // The strip spells out every rule of the active preset, so file types with
  // dedicated settings are visible instead of hiding behind the catch-all.
  let stripRules = $derived.by(() => {
    if (!activePresetSummary?.rules) return [] as string[];
    return activePresetSummary.rules.map((rule) => {
      const parts: string[] = [];
      if (rule.jpegMode && rule.jpegMode !== 'n/a') parts.push(rule.jpegMode === 'lossless' ? 'transcode' : 'lossy');
      if (rule.coreValue && rule.coreValue !== 'n/a') parts.push(rule.coreValue);
      if (rule.effort && rule.effort !== 'default') parts.push(`e ${rule.effort}`);
      const matches = (rule.matches ?? []).join(' / ') || '*';
      return `${matches}: ${parts.join(' · ') || 'cjxl defaults'}`;
    });
  });

  onMount(() => {
    const offFiles = Events.On('files', (event: any) => {
      const incoming = Array.isArray(event?.data) ? event.data : [];
      if (incoming.length === 0) return;
      acceptPaths(incoming);
    });
    const offPreset = Events.On('preset', (event: any) => {
      const selected = String(event?.data ?? '');
      if (selected) {
        presetName = selected;
        void applyPresetCore();
      }
    });
    const offProgress = Events.On('progress', (event: any) => {
      if (event?.data) {
        progress = event.data as ProgressUpdate;
        run.busy = true;
        // The queue is shown inline in the Main/Expert views; only coalesced
        // external invocations use the compact window.
        if (progress.coalesced > 1) {
          view = 'automatic';
        } else if (view === 'presets' || view === 'tools') {
          view = 'main';
        }
      }
    });
    const offFile = Events.On('conversion-file', (event: any) => {
      if (event?.data) {
        const update = event.data as FileUpdate;
        results = [...results, update];
        if (!update.error && !update.skipped && !update.cancelled && update.inputSize >= 0) {
          sessionStats = {
            count: sessionStats.count + 1,
            saved: sessionStats.saved + (update.inputSize - update.outputSize),
          };
        }
      }
    });
    const offToolchainProgress = Events.On('toolchain-progress', (event: any) => {
      if (event?.data) {
        tools.progress = event.data as ToolchainProgress;
      }
    });
    const offDone = Events.On('conversion-done', (event: any) => {
      run.summary = event?.data as ConversionSummary;
      progress = { ...progress, paused: false, percent: 100 };
      run.busy = false;
      collisionPrompt = null; // a cancelled run resolves outstanding prompts itself
      if (run.summary?.cancelled) {
        errorMessage = '! cancelled by user';
      }
    });
    const offError = Events.On('conversion-error', (event: any) => {
      errorMessage = String(event?.data ?? 'Conversion error');
    });
    const offCollision = Events.On('collision-prompt', (event: any) => {
      if (event?.data) collisionPrompt = event.data as CollisionPrompt;
    });

    void load();
    return () => {
      offFiles();
      offPreset();
      offProgress();
      offFile();
      offDone();
      offError();
      offToolchainProgress();
      offCollision();
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
        tools.status = await Service.GetToolchainStatus();
      } catch (error) {
        tools.error = errorText(error);
      }
      try {
        tools.contextMenu = await Service.ContextMenuRegistered();
      } catch (error) {
        errorMessage = errorText(error);
      }
      const pending = (await Service.TakePending()) ?? [];
      const pendingPreset = await Service.TakePendingPreset();
      const activePreset = await Service.GetActivePreset();
      if (pendingPreset) presetName = pendingPreset;
      if (activePreset) presetName = activePreset;
      await applyPresetCore();
      if (pending.length > 0) {
        acceptPaths(pending);
      }
      const runningProgress = await Service.GetProgress();
      if (runningProgress.total > 0) {
        progress = runningProgress;
        view = runningProgress.coalesced > 1 ? 'automatic' : 'main';
        run.busy = true;
      }
      // Recover an output-exists prompt that fired before the subscription.
      collisionPrompt = await Service.GetPendingCollision();
      // Non-blocking: GitHub rate limits/offline just mean no banner.
      void Service.GetAppUpdate()
        .then((update) => { appUpdate = update; })
        .catch(() => {});
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
      processes: settings.processes,
      threads: settings.threads,
      jpegMode: settings.jpegLossless ? 'transcode' : 'reencode',
      distance: settings.distance,
      useDistance: true,
      useQuality: false, // distance is the stored value; -q is a display transform only
      effort: settings.effort,
      useEffort: true,
      outputPolicy: settings.outputPolicy,
      expertFlags: expertOverrides,
      resetExpert: false, // flags live in the preset; Expert edits persist there
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

  async function refreshCommandPreview(): Promise<void> {
    const request = ++commandPreviewRequest;
    if (presetName === '') {
      cmdPreview.previews = [];
      cmdPreview.error = '';
      return;
    }
    try {
      const previews = (await Service.PreviewCommands(currentOptions())) ?? [];
      if (request !== commandPreviewRequest) return;
      cmdPreview.previews = previews;
      cmdPreview.error = '';
    } catch (error) {
      if (request !== commandPreviewRequest) return;
      cmdPreview.previews = [];
      cmdPreview.error = errorText(error);
    }
  }

  function acceptPaths(paths: string[]): void {
    const incoming = paths.filter((path) => path.trim() !== '');
    if (incoming.length === 0) return;
    if (!run.busy) {
      // Adding files after a finished run keeps existing results; only the
      // pending files (no result, or failed/cancelled) are converted next.
      run.summary = null;
      meta.selection = '';
      meta.output = '';
      meta.error = '';
      meta.loading = false;
      metadataRequest += 1;
    }
    const next = [...inputPaths, ...incoming];
    inputPaths = [...new Set(next)];
    errorMessage = '';
    if (view !== 'automatic') {
      view = 'main';
    }
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

  // Drops are native in Wails v3: EnableFileDrop + data-file-drop-target route
  // absolute paths through WindowFilesDropped -> AddPaths -> the "files" event.
  // WebView2 never exposes paths on JS DataTransfer, so no JS drop handler exists.

  async function startConversion(): Promise<void> {
    const runPaths = pendingPaths;
    if (runPaths.length === 0) {
      errorMessage = 'Nothing to convert — all files are already done.';
      return;
    }
    if (presetName === '') {
      errorMessage = 'Select a preset in the toolbar before converting.';
      return;
    }
    if (settings.outputPolicy === 'replace') {
      const preview = await refreshPreview();
      if (preview === null) return;
      const irreversible = preview.filter((file) => file.route === 'Reencode' || (file.route === 'Encode' && settings.distance > 0)).length;
      if (irreversible > 0 && !window.confirm(`Replace the originals for ${irreversible} irreversible file${irreversible === 1 ? '' : 's'}? They will be sent to the recycle bin after verification.`)) {
        return;
      }
    }
    run.busy = true;
    errorMessage = '';
    meta.selection = '';
    meta.output = '';
    meta.error = '';
    meta.loading = false;
    metadataRequest += 1;
    run.summary = null;
    progress = { ...progress, total: runPaths.length, completed: 0, failed: 0, skipped: 0, inFlight: 0, percent: 0, paused: false };
    try {
      await Service.StartConversion(runPaths, currentOptions());
    } catch (error) {
      run.busy = false;
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
    tools.error = '';
    try {
      tools.status = await Service.GetToolchainStatus();
    } catch (error) {
      tools.error = errorText(error);
    }
  }

  async function installToolchain(): Promise<void> {
    tools.installing = true;
    tools.progress = null;
    tools.error = '';
    try {
      await Service.InstallLatestToolchain();
      await refreshToolchain();
    } catch (error) {
      tools.error = errorText(error);
    } finally {
      tools.installing = false;
      tools.progress = null;
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
      await applyPresetCore();
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
    const selected = presets.find((preset) => preset.name === name);
    presetPolicyDraft = selected?.policy ?? '';
    presetCollisionDraft = selected?.collision ?? 'skip';
  }

  async function savePresetOutput(): Promise<void> {
    if (!selectedPreset || selectedPresetReadOnly || !presetOutputDirty) return;
    try {
      await Service.SavePresetOutput(selectedPreset, presetPolicyDraft, presetCollisionDraft);
      await refreshPresets();
      selectPreset(selectedPreset);
      // A saved output block changes what the GUI controls show for this preset.
      if (selectedPreset === presetName) await applyPresetCore();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function openPresetInEditor(): Promise<void> {
    if (!selectedPreset) return;
    try {
      await Service.OpenPresetInEditor(selectedPreset);
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
        await applyPresetCore();
      }
      appStatus = await Service.GetStatus();
      if (entryPoint === 'contextmenu') {
        tools.contextMenu = await Service.ContextMenuRegistered();
      }
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function registerContextMenu(): Promise<void> {
    try {
      await Service.RegisterContextMenu();
      tools.contextMenu = await Service.ContextMenuRegistered();
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function unregisterContextMenu(): Promise<void> {
    try {
      await Service.UnregisterContextMenu();
      tools.contextMenu = false;
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

  function clearAll(): void {
    inputPaths = [];
    files = [];
    previewRequest += 1;
    cmdPreview.previews = [];
    cmdPreview.error = '';
    commandPreviewRequest += 1;
    results = [];
    meta.selection = '';
    meta.output = '';
    meta.error = '';
    meta.loading = false;
    metadataRequest += 1;
    run.summary = null;
    progress = { ...progress, total: 0, completed: 0, failed: 0, skipped: 0, percent: 0 };
    view = 'main';
  }

  async function loadHistory(): Promise<void> {
    try {
      history.entries = (await Service.GetHistoryEntries()) ?? [];
      history.loaded = true;
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  function openHistory(): void {
    view = 'history';
    history.loaded = false;
    history.meta = { entry: null, output: '', error: '', loading: false };
    void loadHistory();
  }

  async function clearHistoryAll(): Promise<void> {
    if (!window.confirm(`Remove all ${history.entries.length} history entries? The converted files are not touched.`)) return;
    try {
      await Service.ClearHistory();
      history.entries = [];
      history.meta = { entry: null, output: '', error: '', loading: false };
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function inspectHistoryEntry(entry: HistoryEntry): Promise<void> {
    const request = ++historyMetaRequest;
    history.meta = { entry, output: '', error: '', loading: true };
    try {
      const output = await Service.InspectJXL(entry.output);
      if (request !== historyMetaRequest) return;
      history.meta = { entry, output, error: '', loading: false };
    } catch (error) {
      if (request !== historyMetaRequest) return;
      history.meta = { entry, output: '', error: errorText(error), loading: false };
    }
  }

  function resolveCollision(action: string): void {
    collisionPrompt = null;
    void Service.ResolveCollision(action).catch((error: unknown) => {
      errorMessage = errorText(error);
    });
  }

  // Selecting a preset snapshots its core (applyPresetCore). GUI edits are
  // session-only: preview and conversion run with the overrides, the preset
  // file is never touched, and presetChanged marks the strip. Revert restores
  // the snapshot; persisting happens by editing the YAML (Presets view).
  interface CoreSnapshot {
    distance: number;
    effort: number;
    jpegLossless: boolean;
    policy: string;
    flags: FlagOverride[];
  }

  let coreSnapshot = $state<CoreSnapshot | null>(null);

  function applyCore(core: {
    distance: number;
    effort: number;
    jpegMode: string;
    policy: string;
    flags: FlagOverride[] | null;
  }): CoreSnapshot {
    const flags = core.flags ?? [];
    settings.distance = core.distance;
    settings.effort = core.effort;
    routeMode = core.distance === 0 ? 'lossless' : 'lossy';
    if (core.distance > 0) settings.lossyDistance = core.distance;
    settings.jpegLossless = core.jpegMode !== 'reencode';
    settings.outputPolicy = core.policy || 'alongside';
    expertOverrides = flags.map((flag) => ({ ...flag }));
    return {
      distance: core.distance,
      effort: core.effort,
      jpegLossless: settings.jpegLossless,
      policy: settings.outputPolicy,
      flags,
    };
  }

  async function applyPresetCore(): Promise<void> {
    if (presetName === '') {
      coreSnapshot = null;
      return;
    }
    try {
      coreSnapshot = applyCore(await Service.GetPresetCore(presetName));
    } catch (error) {
      errorMessage = errorText(error);
    }
    if (!run.busy) void refreshPreview();
  }

  function revertToPreset(): void {
    if (!coreSnapshot) return;
    applyCore({
      distance: coreSnapshot.distance,
      effort: coreSnapshot.effort,
      jpegMode: coreSnapshot.jpegLossless ? 'transcode' : 'reencode',
      policy: coreSnapshot.policy,
      flags: coreSnapshot.flags,
    });
    onSettingsChanged();
  }

  function onSettingsChanged(): void {
    void refreshCommandPreview();
    if (run.busy) return; // running engine keeps its start-time snapshot; don't relabel files
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => void refreshPreview(), 300);
  }

  function setDistanceValue(value: number): void {
    settings.distance = value;
    if (routeMode === 'lossy') settings.lossyDistance = settings.distance;
    onSettingsChanged();
  }

  function setQualityValue(value: number): void {
    settings.distance = distanceFromQuality(value);
    if (routeMode === 'lossy') settings.lossyDistance = settings.distance;
    onSettingsChanged();
  }

  function setEffortValue(value: number): void {
    settings.effort = value;
    onSettingsChanged();
  }

  function setExpertFlagValue(key: string, value: string): void {
    const next = expertOverrides.filter((override) => override.key !== key);
    if (value.trim() !== '') {
      next.push({ key, value, valueless: false });
    }
    expertOverrides = next;
    onSettingsChanged();
  }

  function setExpertFlagEnabled(key: string, enabled: boolean): void {
    const next = expertOverrides.filter((override) => override.key !== key);
    if (enabled) {
      next.push({ key, value: '', valueless: true });
    }
    expertOverrides = next;
    onSettingsChanged();
  }

  function resetExpertFlags(): void {
    expertOverrides = [];
    onSettingsChanged();
  }

  function setRouteMode(next: RouteMode): void {
    if (next === routeMode) return;
    if (next === 'lossless') {
      settings.lossyDistance = settings.distance > 0 ? settings.distance : settings.lossyDistance;
      settings.distance = 0;
    } else {
      settings.distance = settings.lossyDistance > 0 ? settings.lossyDistance : 1.0;
    }
    routeMode = next;
    onSettingsChanged();
  }

  async function selectResult(result: FileUpdate): Promise<void> {
    meta.selection = result.input;
    meta.output = '';
    meta.error = '';
    meta.loading = true;
    const request = ++metadataRequest;
    if (result.error || result.skipped || result.cancelled || !result.output) {
      meta.error = result.error || result.skipReason || (result.cancelled ? 'Conversion was cancelled.' : 'No JXL output was produced for this result.');
      meta.loading = false;
      return;
    }
    try {
      const output = await Service.InspectJXL(result.output);
      if (request !== metadataRequest) return;
      meta.output = output;
    } catch (error) {
      if (request !== metadataRequest) return;
      meta.error = errorText(error);
    } finally {
      if (request === metadataRequest) meta.loading = false;
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

<main class:narrow={view === 'automatic'} class="app" data-file-drop-target>
  <div class="toolbar">
    <div class="seg" aria-label="Editor mode">
      <button aria-pressed={view === 'main'} onclick={() => { view = 'main'; }}>Main</button>
      <button aria-pressed={view === 'expert'} onclick={() => { view = 'expert'; }}>Expert</button>
    </div>
    <div class="field">
      <label for="preset-select">Preset</label>
      <select id="preset-select" bind:value={presetName} onchange={() => void applyPresetCore()} disabled={presets.length === 0}>
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
    <button class="btn ghost" onclick={() => { view = 'presets'; }}>Presets</button>
    <button class="btn ghost" onclick={() => { view = 'tools'; }}>Tools</button>
    <button class="btn ghost" onclick={openHistory}>History</button>
    <span class="spacer"></span>
    {#if view === 'presets' || view === 'tools' || view === 'history'}
      <button class="btn" onclick={() => { view = 'main'; }}>Back</button>
    {/if}
  </div>

  {#if (view === 'main' || view === 'expert') && presetName !== ''}
    <div class="preset-strip" class:strip-dirty={presetChanged} data-testid="preset-strip">
      <span class="strip-label">Preset</span>
      <strong>{presetName}</strong>
      {#if activePresetSummary?.readOnly}<span class="mini" title="Factory preset; it cannot be modified">read-only</span>{/if}
      {#each stripRules as rule}
        <code class="rule-chip">{rule}</code>
      {/each}
      <span class="mini">output: {settings.outputPolicy}</span>
      {#if presetChanged}
        <span class="spacer"></span>
        <span class="strip-warning" role="status">! settings differ — preset “{presetName}” is not in effect</span>
        <button class="btn ghost strip-btn" data-testid="revert-preset" onclick={revertToPreset}>Revert</button>
      {/if}
    </div>
  {/if}

  {#if errorMessage}
    <div class="banner warn" role="alert">
      <span class="ic">!</span><span>{errorMessage}</span>
      <button class="alert-close" aria-label="Dismiss message" title="Dismiss" onclick={dismissMessage}>x</button>
    </div>
  {/if}

  {#if appUpdate?.available}
    <div class="banner warn" role="status" data-testid="app-update">
      <span class="ic">!</span>
      <span>jxleet {appUpdate.latest} is available on GitHub — you are running {appUpdate.current}.</span>
      <button class="btn" style="margin-left:auto" onclick={() => void Service.OpenURL(appUpdate?.url ?? '')}>Open release page</button>
      <button class="alert-close" aria-label="Dismiss update notice" title="Dismiss" onclick={() => { appUpdate = null; }}>x</button>
    </div>
  {/if}

  {#if collisionPrompt}
    <div class="banner warn collision-banner" role="alertdialog" aria-label="Output file already exists" data-testid="collision-prompt">
      <span class="ic">!</span>
      <div class="collision-copy">
        <b>The JXL output already exists.</b>
        <div class="mini" title={collisionPrompt.input}>{collisionPrompt.input.split(/[\\/]/).pop()} -&gt; {collisionPrompt.output}</div>
      </div>
      <div class="collision-actions">
        <button class="btn primary" style="background:var(--p-encode)" data-testid="collision-overwrite" onclick={() => resolveCollision('overwrite')}>Overwrite</button>
        <button class="btn" data-testid="collision-overwrite-all" onclick={() => resolveCollision('overwrite-all')}>Overwrite all</button>
        <button class="btn" data-testid="collision-skip" onclick={() => resolveCollision('skip')}>Skip file</button>
        <button class="btn" data-testid="collision-skip-all" onclick={() => resolveCollision('skip-all')}>Skip all</button>
      </div>
    </div>
  {/if}

  {#if !loaded}
    <div class="body"><div class="empty">Loading jxleet...</div></div>
  {:else if view === 'main'}
    <MainView
      files={files}
      results={results}
      meta={meta}
      run={run}
      progress={progress}
      settings={settings}
      routeMode={routeMode}
      quality={quality}
      outOfRange={outOfRange}
      presetName={presetName}
      appStatus={appStatus}
      tools={tools}
      canConvert={canConvert}
      pendingCount={pendingPaths.length}
      onOpenFile={() => void openFile()}
      onOpenFolder={() => void openFolder()}
      onTogglePause={() => void togglePause()}
      onCancel={() => void cancelConversion()}
      onInstallToolchain={() => void installToolchain()}
      onGoToPresets={() => { view = 'presets'; }}
      onSelectResult={(result) => void selectResult(result)}
      onClearAll={clearAll}
      onSetDistance={setDistanceValue}
      onSetQuality={setQualityValue}
      onSetEffort={setEffortValue}
      onSetJpegMode={(lossless) => { settings.jpegLossless = lossless; onSettingsChanged(); }}
      onSetOutputPolicy={(policy) => { settings.outputPolicy = policy; onSettingsChanged(); }}
      onStart={() => void startConversion()}
    />
  {:else if view === 'expert'}
    <ExpertView
      routeMode={routeMode}
      presetName={presetName}
      settings={settings}
      quality={quality}
      outOfRange={outOfRange}
      previews={cmdPreview.previews}
      previewError={cmdPreview.error}
      flagDefinitions={flagDefinitions}
      expertOverrides={expertOverrides}
      flagsLocked={Boolean(tools.status?.flagsLocked)}
      canConvert={canConvert}
      pendingCount={pendingPaths.length}
      filesCount={files.length}
      onSetRouteMode={setRouteMode}
      onSetEffort={setEffortValue}
      onSetDistance={setDistanceValue}
      onSetQuality={setQualityValue}
      onResetFlags={resetExpertFlags}
      onSetFlagValue={setExpertFlagValue}
      onSetFlagEnabled={setExpertFlagEnabled}
      onStart={() => void startConversion()}
    />
  {:else if view === 'automatic'}
    <AutomaticView
      presetName={presetName}
      processes={settings.processes}
      threads={settings.threads}
      progress={progress}
      results={results}
      files={files}
      onTogglePause={() => void togglePause()}
      onCancel={() => void cancelConversion()}
    />
  {:else if view === 'tools'}
    <ToolsView
      tools={tools}
      bindings={bindings}
      onRefresh={() => void refreshToolchain()}
      onInstall={() => void installToolchain()}
      onRegister={() => void registerContextMenu()}
      onUnregister={() => void unregisterContextMenu()}
      onOpenStorage={(location) => void openStorage(location)}
    />
  {:else if view === 'presets'}
    <PresetsView
      presets={presets}
      selectedPreset={selectedPreset}
      bindings={bindings}
      selectedData={selectedPresetData}
      readOnly={selectedPresetReadOnly}
      outputDirty={presetOutputDirty}
      bind:policyDraft={presetPolicyDraft}
      bind:collisionDraft={presetCollisionDraft}
      onSelect={selectPreset}
      onCreate={() => void createPreset()}
      onDuplicate={() => void duplicatePreset()}
      onRename={() => void renamePreset()}
      onDelete={() => void deletePreset()}
      onOpenInEditor={() => void openPresetInEditor()}
      onReload={() => void refreshPresets()}
      onOpenStorage={(location) => void openStorage(location)}
      onSaveOutput={() => void savePresetOutput()}
      onSetBinding={(entryPoint, value) => void setBinding(entryPoint, value)}
    />
  {:else if view === 'history'}
    <HistoryView
      entries={history.entries}
      loaded={history.loaded}
      meta={history.meta}
      onReload={() => void loadHistory()}
      onClear={() => void clearHistoryAll()}
      onInspect={(entry) => void inspectHistoryEntry(entry)}
    />
  {/if}

  <div class="statusbar">
    {#if tools.status?.installedVersion}
      <span class="ok"><span class="dotled"></span>cjxl {tools.status.installedVersion}</span>
    {:else}
      <span class="warning"><span class="dotled"></span>libjxl not installed</span>
    {/if}
    <span class="spacer"></span>
    {#if tools.installing && tools.progress}
      <span>{tools.progress.phase === 'downloading' ? 'Downloading libjxl' : 'Installing libjxl'}{#if tools.progress.phase === 'downloading' && tools.progress.total > 0} — {formatBytes(tools.progress.downloaded)} / {formatBytes(tools.progress.total)}{/if}</span>
    {:else if run.busy || view === 'automatic'}
      <span>{progress.completed} done - {progress.failed} failed</span><span>{formatRate(progress.throughput)}</span>
    {:else if view === 'tools' && tools.status?.updateAvailable}
      <span class="up">Update available</span>
    {:else if files.length > 0}
      <span>{files.length} files - {formatBytes(totalSize)}</span>
    {:else}
      <span>Queue empty</span>
    {/if}
    {#if sessionStats.count > 0}
      <span class="mini">{sessionStats.count} converted · {formatBytes(Math.abs(sessionStats.saved))} {sessionStats.saved >= 0 ? 'saved' : 'grew'}</span>
    {/if}
    <span>1 instance</span>
  </div>
</main>
