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
  import type { FileGroup, RouteMode, View } from './lib/types';
  import { effortNames, effortTools, stageIndexAt } from './lib/effort';
  import {
    distanceFromQuality,
    formatDistance,
    qualityFromDistance,
    qualityStatusText,
  } from './lib/quality';
  import { compactPath, formatBytes, formatDelta, formatEta, formatRate, savedPct } from './lib/format';
  import { routeClass, routeTitle } from './lib/routes';
  import { flagLabel, hiddenExpertFlags, isLinkedFlagKey, sameFlags } from './lib/flags';
  import EffortLadder from './components/EffortLadder.svelte';
  import QualitySliders from './components/QualitySliders.svelte';
  import CommandPreviewPanel from './components/CommandPreview.svelte';
  import JxlInfoPanel from './components/JxlInfoPanel.svelte';
  import AutomaticView from './views/AutomaticView.svelte';
  import ToolsView from './views/ToolsView.svelte';
  import HistoryView from './views/HistoryView.svelte';
  import PresetsView from './views/PresetsView.svelte';


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
  let collapsedGroups = $state(new Set<string>());
  let loaded = $state(false);
  let sessionStats = $state({ count: 0, saved: 0 });
  let collisionPrompt = $state<CollisionPrompt | null>(null);
  let presetPolicyDraft = $state('');
  let presetCollisionDraft = $state('skip');

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
  let presetOutputDirty = $derived(
    Boolean(selectedPresetData) &&
      (presetPolicyDraft !== selectedPresetData?.policy || presetCollisionDraft !== selectedPresetData?.collision),
  );
  let dominantRoute = $derived(
    routeCounts.Reencode >= routeCounts.Encode && routeCounts.Reencode >= routeCounts.Transcode
      ? 'Reencode'
      : routeCounts.Encode >= routeCounts.Transcode
        ? 'Encode'
        : 'Transcode',
  );
  let quality = $derived(Math.round(qualityFromDistance(settings.distance)));
  let outOfRange = $derived(routeMode !== 'lossless' && (settings.distance < 0.5 || settings.distance > 3));
  let canConvert = $derived(inputPaths.length > 0 && files.length > 0 && presetName !== '' && !run.busy && !tools.installing);
  let selectedResult = $derived(results.find((result) => result.input === meta.selection) ?? null);
  let processing = $derived(run.busy && (view === 'main' || view === 'expert'));
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

  function setEffort(event: Event): void {
    setEffortValue(Number((event.currentTarget as HTMLInputElement).value));
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

  function isLinkedFlag(flag: FlagInfo): boolean {
    return isLinkedFlagKey(flag.key);
  }

  function linkedFlagValue(flag: FlagInfo): string {
    switch (flag.key) {
      case '--distance':
        return formatDistance(settings.distance);
      case '--quality':
        return quality.toString();
      case '--effort':
        return settings.effort.toString();
      case '--lossless_jpeg':
        return settings.jpegLossless ? '1' : '0';
      case '--num_threads':
        return settings.threads.toString();
      default:
        return '';
    }
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
    <div class="body">
      {#if tools.installing}
        <div class="run-strip" data-testid="install-strip">
          <div class="run-head">
            <span class="badge b-encode">Installing</span>
            <span class="run-count">{tools.progress?.phase === 'downloading' ? 'Downloading libjxl' : 'Verifying & tools.installing'}</span>
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
          <button class="btn primary" style="margin-left:auto;background:var(--p-encode)" onclick={() => void installToolchain()} disabled={tools.installing}>Install</button>
        </div>
      {/if}
      {#if appStatus && !appStatus.ready}
        <div class="banner info">
          <span class="ic">i</span>
          <span>Bind a preset for each entry point before automated runs.</span>
          <button class="btn ghost" style="margin-left:auto" onclick={() => { view = 'presets'; }}>Set bindings</button>
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
          <button class="btn primary" style="background:var(--p-encode)" onclick={() => void openFile()}>Open File</button>
          <button class="btn" onclick={() => void openFolder()}>Open Folder</button>
        </div>
      </div>
      {:else}
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
                        onclick={() => { if (inspectable && result) void selectResult(result); }}
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
              <QualitySliders distance={settings.distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} onDistance={setDistanceValue} onQuality={setQualityValue} />
              <div class="effort-basic" style="border-top:1px solid var(--line-soft);margin-top:8px;padding-top:8px">
                <div style="display:flex;align-items:baseline;gap:8px">
                  <span class="k">Effort</span>
                  <span class="v">{settings.effort} - {effortNames[settings.effort - 1]}</span>
                  <span class="mini" style="margin-left:auto">{settings.effort === 7 ? 'default' : settings.effort >= 9 ? 'slow' : settings.effort <= 3 ? 'fast' : ''}</span>
                </div>
                <input type="range" min="1" max="10" step="1" value={settings.effort} oninput={setEffort} data-testid="effort-range-basic" aria-label="Effort" />
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
                <input type="radio" name="jpeg-mode" checked={settings.jpegLossless} onchange={() => { settings.jpegLossless = true; onSettingsChanged(); }} />
                <span><span class="ot">Transcode</span><span class="od">Bit-exactly reversible; the original JPEG can be reconstructed.</span></span>
              </label>
              <label class="opt" data-sel={!settings.jpegLossless}>
                <input type="radio" name="jpeg-mode" checked={!settings.jpegLossless} onchange={() => { settings.jpegLossless = false; onSettingsChanged(); }} />
                <span><span class="ot">Reencode</span><span class="od">Uses settings.distance and settings.effort; not reversible.</span></span>
              </label>
              <div class="mini" style="padding:6px 8px 0;border-top:1px solid var(--line-soft);margin-top:4px">--lossless_jpeg={settings.jpegLossless ? 1 : 0}</div>
            </div>
          </div>

          <div class="card">
            <h3>Output</h3>
            <div class="in policy" data-testid="output-policy">
              <label class="opt" data-sel={settings.outputPolicy === 'alongside'}>
                <input type="radio" name="output" checked={settings.outputPolicy === 'alongside'} onchange={() => { settings.outputPolicy = 'alongside'; onSettingsChanged(); }} />
                <span><span class="ot">Alongside</span><span class="od">The original stays untouched.</span></span>
              </label>
              <label class="opt" data-sel={settings.outputPolicy === 'subfolder'}>
                <input type="radio" name="output" checked={settings.outputPolicy === 'subfolder'} onchange={() => { settings.outputPolicy = 'subfolder'; onSettingsChanged(); }} />
                <span><span class="ot">Into subfolder</span><span class="od">./jxl/ relative to the source.</span></span>
              </label>
              <label class="opt risk" data-sel={settings.outputPolicy === 'replace'}>
                <input type="radio" name="output" checked={settings.outputPolicy === 'replace'} onchange={() => { settings.outputPolicy = 'replace'; onSettingsChanged(); }} />
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
        {#if !run.busy}<button class="btn" data-testid="new-files" onclick={clearAll}>Clear All</button>{/if}
        <button class="btn primary convert-action" style={`background:var(--p-${dominantRoute === 'Transcode' ? 'transcode' : dominantRoute === 'Encode' ? 'encode' : 'reencode'});padding:11px`} data-testid="start-convert" onclick={() => void startConversion()} disabled={!canConvert}>Convert {pendingPaths.length || files.length} files</button>
      </div>
    {/if}
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
        <span class="mini">Editing preset: {presetName || 'none'}</span>
      </div>
      <div class="cols wide-right">
        <div class="card">
          <h3>Effort <span class="r">what this level adds</span></h3>
          <div class="in">
            <EffortLadder effort={settings.effort} routeMode={routeMode} onEffortInput={setEffortValue} />
            <CommandPreviewPanel previews={cmdPreview.previews} error={cmdPreview.error} />
          </div>
        </div>

        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Quality</h3>
            <div class="in">
              <QualitySliders distance={settings.distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} locked={routeMode === 'lossless'} onDistance={setDistanceValue} onQuality={setQualityValue} />
              <div class="banner info" style="margin:10px 0 0"><span class="ic">i</span><span>Distance and quality control the same quantity; moving either slider moves the other. The run always uses -d.</span></div>
            </div>
          </div>
          <div class="card">
            <h3>Further flags <span class="r">{visibleFlagCount} cjxl flags</span></h3>
            <div class="in">
              <div class="banner info flag-notice">
                <span class="ic">i</span>
                <span>Changes apply to this session only and are not saved to the preset. Persist them in the YAML file (Presets → Open in Editor); Reset restores the preset values.</span>
              </div>
              {#if tools.status?.flagsLocked}
                <div class="banner warn"><span class="ic">!</span><span>Expert flags are locked because the installed cjxl version differs from the generated help.</span></div>
              {/if}
              <div class="flag-actions">
                <button class="btn ghost" onclick={resetExpertFlags} disabled={Boolean(tools.status?.flagsLocked)}>Reset Expert flags</button>
                <span class="mini">Reset removes the extra flags stored in the preset so cjxl defaults apply.</span>
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
                          disabled={linked || Boolean(tools.status?.flagsLocked)}
                          oninput={(event) => setExpertFlagValue(flag.key, (event.currentTarget as HTMLInputElement).value)}
                        />
                      {:else}
                        <input
                          class="flag-toggle"
                          type="checkbox"
                          checked={expertFlagEnabled(flag.key)}
                          aria-label={flagLabel(flag)}
                          title={flag.description}
                          disabled={Boolean(tools.status?.flagsLocked)}
                          onchange={(event) => setExpertFlagEnabled(flag.key, (event.currentTarget as HTMLInputElement).checked)}
                        />
                      {/if}
                    </div>
                  {/each}
                </div>
              {/each}
            </div>
          </div>
          <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" onclick={() => void startConversion()} disabled={!canConvert}>Convert {pendingPaths.length || files.length} files</button>
        </div>
      </div>
    </div>
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
