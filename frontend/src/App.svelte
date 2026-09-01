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
  let selectedResultInput = $state('');
  let metadataOutput = $state('');
  let metadataError = $state('');
  let metadataLoading = $state(false);
  let metadataRequest = 0;
  let previewRequest = 0;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
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
  let appUpdate = $state<Update | null>(null);
  let toolchain = $state<ToolchainStatus | null>(null);
  let toolchainError = $state('');
  let contextMenuRegistered = $state(false);
  let errorMessage = $state('');
  let collapsedGroups = $state(new Set<string>());
  let loaded = $state(false);
  let busy = $state(false);
  let installing = $state(false);
  let installProgress = $state<ToolchainProgress | null>(null);
  let sessionStats = $state({ count: 0, saved: 0 });
  let historyEntries = $state<HistoryEntry[]>([]);
  let historyLoaded = $state(false);
  let historyMeta = $state<{ entry: HistoryEntry | null; output: string; error: string; loading: boolean }>({
    entry: null,
    output: '',
    error: '',
    loading: false,
  });
  let historyMetaRequest = 0;
  let collisionPrompt = $state<CollisionPrompt | null>(null);
  let distance = $state(1.0);
  let effort = $state(7);
  let jpegLossless = $state(true);
  let outputPolicy = $state('alongside');
  let processes = $state(2);
  let threads = $state(8);
  let presetPolicyDraft = $state('');
  let presetCollisionDraft = $state('skip');
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
  let quality = $derived(Math.round(qualityFromDistance(distance)));
  let outOfRange = $derived(routeMode !== 'lossless' && (distance < 0.5 || distance > 3));
  let canConvert = $derived(inputPaths.length > 0 && files.length > 0 && presetName !== '' && !busy && !installing);
  let selectedResult = $derived(results.find((result) => result.input === selectedResultInput) ?? null);
  let processing = $derived(busy && (view === 'main' || view === 'expert'));
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
      distance !== snapshot.distance ||
      effort !== snapshot.effort ||
      jpegLossless !== snapshot.jpegLossless ||
      outputPolicy !== snapshot.policy ||
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
        busy = true;
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
        installProgress = event.data as ToolchainProgress;
      }
    });
    const offDone = Events.On('conversion-done', (event: any) => {
      summary = event?.data as ConversionSummary;
      progress = { ...progress, paused: false, percent: 100 };
      busy = false;
      collisionPrompt = null; // a cancelled run resolves outstanding prompts itself
      if (summary?.cancelled) {
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
      await applyPresetCore();
      if (pending.length > 0) {
        acceptPaths(pending);
      }
      const runningProgress = await Service.GetProgress();
      if (runningProgress.total > 0) {
        progress = runningProgress;
        view = runningProgress.coalesced > 1 ? 'automatic' : 'main';
        busy = true;
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
      processes,
      threads,
      jpegMode: jpegLossless ? 'transcode' : 'reencode',
      distance,
      useDistance: true,
      useQuality: false, // distance is the stored value; -q is a display transform only
      effort,
      useEffort: true,
      outputPolicy,
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
    if (!busy) {
      // Adding files after a finished run keeps existing results; only the
      // pending files (no result, or failed/cancelled) are converted next.
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
    selectedResultInput = '';
    metadataOutput = '';
    metadataError = '';
    metadataLoading = false;
    metadataRequest += 1;
    summary = null;
    progress = { ...progress, total: runPaths.length, completed: 0, failed: 0, skipped: 0, inFlight: 0, percent: 0, paused: false };
    try {
      await Service.StartConversion(runPaths, currentOptions());
    } catch (error) {
      busy = false;
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
    installing = true;
    installProgress = null;
    toolchainError = '';
    try {
      await Service.InstallLatestToolchain();
      await refreshToolchain();
    } catch (error) {
      toolchainError = errorText(error);
    } finally {
      installing = false;
      installProgress = null;
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

  function clearAll(): void {
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
    view = 'main';
  }

  async function loadHistory(): Promise<void> {
    try {
      historyEntries = (await Service.GetHistoryEntries()) ?? [];
      historyLoaded = true;
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  function openHistory(): void {
    view = 'history';
    historyLoaded = false;
    historyMeta = { entry: null, output: '', error: '', loading: false };
    void loadHistory();
  }

  async function clearHistoryAll(): Promise<void> {
    if (!window.confirm(`Remove all ${historyEntries.length} history entries? The converted files are not touched.`)) return;
    try {
      await Service.ClearHistory();
      historyEntries = [];
      historyMeta = { entry: null, output: '', error: '', loading: false };
    } catch (error) {
      errorMessage = errorText(error);
    }
  }

  async function inspectHistoryEntry(entry: HistoryEntry): Promise<void> {
    const request = ++historyMetaRequest;
    historyMeta = { entry, output: '', error: '', loading: true };
    try {
      const output = await Service.InspectJXL(entry.output);
      if (request !== historyMetaRequest) return;
      historyMeta = { entry, output, error: '', loading: false };
    } catch (error) {
      if (request !== historyMetaRequest) return;
      historyMeta = { entry, output: '', error: errorText(error), loading: false };
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
    distance = core.distance;
    effort = core.effort;
    routeMode = core.distance === 0 ? 'lossless' : 'lossy';
    if (core.distance > 0) lossyDistance = core.distance;
    jpegLossless = core.jpegMode !== 'reencode';
    outputPolicy = core.policy || 'alongside';
    expertOverrides = flags.map((flag) => ({ ...flag }));
    return {
      distance: core.distance,
      effort: core.effort,
      jpegLossless,
      policy: outputPolicy,
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
    if (!busy) void refreshPreview();
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
    if (busy) return; // running engine keeps its start-time snapshot; don't relabel files
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
    distance = value;
    if (routeMode === 'lossy') lossyDistance = distance;
    onSettingsChanged();
  }

  function setQualityValue(value: number): void {
    distance = distanceFromQuality(value);
    if (routeMode === 'lossy') lossyDistance = distance;
    onSettingsChanged();
  }

  function setEffortValue(value: number): void {
    effort = value;
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
        return formatDistance(distance);
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

  function resetExpertFlags(): void {
    expertOverrides = [];
    onSettingsChanged();
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
    onSettingsChanged();
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
      <span class="mini">output: {outputPolicy}</span>
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
      {#if installing}
        <div class="run-strip" data-testid="install-strip">
          <div class="run-head">
            <span class="badge b-encode">Installing</span>
            <span class="run-count">{installProgress?.phase === 'downloading' ? 'Downloading libjxl' : 'Verifying & installing'}</span>
            {#if installProgress?.phase === 'downloading'}
              <span class="mini">{formatBytes(installProgress.downloaded)} of {installProgress.total > 0 ? formatBytes(installProgress.total) : '?'}</span>
            {/if}
          </div>
          {#if installProgress?.phase === 'downloading' && installProgress.total > 0}
            <div class="bar"><i style={`width:${Math.min(100, (installProgress.downloaded / installProgress.total) * 100)}%`}></i></div>
          {/if}
        </div>
      {:else if toolchain?.needsInstall}
        <div class="banner warn">
          <span class="ic">!</span>
          <span><b>libjxl is not installed.</b> Install the managed cjxl/djxl/jxlinfo toolchain before converting.</span>
          <button class="btn primary" style="margin-left:auto;background:var(--p-encode)" onclick={() => void installToolchain()} disabled={installing}>Install</button>
        </div>
      {/if}
      {#if appStatus && !appStatus.ready}
        <div class="banner info">
          <span class="ic">i</span>
          <span>Bind a preset for each entry point before automated runs.</span>
          <button class="btn ghost" style="margin-left:auto" onclick={() => { view = 'presets'; }}>Set bindings</button>
        </div>
      {/if}
      {#if files.length === 0 && !busy}
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
                      {@const inspectable = summary != null && !busy && result != null && !failed && !result.skipped && !result.cancelled}
                      <tr
                        class:selected={result != null && selectedResultInput === result.input}
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
                          {:else if busy}
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
              <QualitySliders distance={distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} onDistance={setDistanceValue} onQuality={setQualityValue} />
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
                <input type="radio" name="jpeg-mode" checked={jpegLossless} onchange={() => { jpegLossless = true; onSettingsChanged(); }} />
                <span><span class="ot">Transcode</span><span class="od">Bit-exactly reversible; the original JPEG can be reconstructed.</span></span>
              </label>
              <label class="opt" data-sel={!jpegLossless}>
                <input type="radio" name="jpeg-mode" checked={!jpegLossless} onchange={() => { jpegLossless = false; onSettingsChanged(); }} />
                <span><span class="ot">Reencode</span><span class="od">Uses distance and effort; not reversible.</span></span>
              </label>
              <div class="mini" style="padding:6px 8px 0;border-top:1px solid var(--line-soft);margin-top:4px">--lossless_jpeg={jpegLossless ? 1 : 0}</div>
            </div>
          </div>

          <div class="card">
            <h3>Output</h3>
            <div class="in policy" data-testid="output-policy">
              <label class="opt" data-sel={outputPolicy === 'alongside'}>
                <input type="radio" name="output" checked={outputPolicy === 'alongside'} onchange={() => { outputPolicy = 'alongside'; onSettingsChanged(); }} />
                <span><span class="ot">Alongside</span><span class="od">The original stays untouched.</span></span>
              </label>
              <label class="opt" data-sel={outputPolicy === 'subfolder'}>
                <input type="radio" name="output" checked={outputPolicy === 'subfolder'} onchange={() => { outputPolicy = 'subfolder'; onSettingsChanged(); }} />
                <span><span class="ot">Into subfolder</span><span class="od">./jxl/ relative to the source.</span></span>
              </label>
              <label class="opt risk" data-sel={outputPolicy === 'replace'}>
                <input type="radio" name="output" checked={outputPolicy === 'replace'} onchange={() => { outputPolicy = 'replace'; onSettingsChanged(); }} />
                <span><span class="ot">Replace, original to recycle bin</span><span class="od">Only after verification. Irreversible routes require confirmation.</span></span>
              </label>
            </div>
          </div>

          {#if summary && !busy}
            <div class="card">
              <h3>Output summary</h3>
              <div class="in kv">
                <div><span>Converted</span><span>{summary.completed}{#if summary.failed > 0} - {summary.failed} failed{/if}{#if summary.skipped > 0} - {summary.skipped} skipped{/if}</span></div>
                <div><span>Bytes</span><span>{formatBytes(summary.bytesIn)} -&gt; {formatBytes(summary.bytesOut)}</span></div>
                <div><span>Saved</span><span class="success">{formatDelta(summary.bytesIn, summary.bytesOut)}</span></div>
              </div>
            </div>
            <JxlInfoPanel
              label={selectedResult ? (selectedResult.output ? compactPath(selectedResult.output, 34) : 'unavailable') : 'select a converted file'}
              emptyText="Select a converted file to inspect its JPEG XL metadata."
              hasSelection={selectedResult != null}
              loading={metadataLoading}
              error={metadataError}
              output={metadataOutput}
            />
          {/if}
        </div>
      </div>
      {/if}
      {/if}
    </div>
    {#if files.length > 0}
      <div class="convertbar" data-testid="convertbar">
        {#if summary && !busy}
          <span class:error={summary.failed > 0} class:success={summary.failed === 0}>{summary.completed} converted{#if summary.failed > 0} - {summary.failed} failed{/if}{#if summary.skipped > 0} - {summary.skipped} skipped{/if}</span>
          <span class="mono-mini">{formatBytes(summary.bytesIn)} -&gt; {formatBytes(summary.bytesOut)}</span>
          <span class="delta-chip" class:neg={savedPct(summary.bytesIn, summary.bytesOut) < 0}>{formatDelta(summary.bytesIn, summary.bytesOut)}</span>
        {:else}
          <span class="mini">{files.length} files - {formatBytes(totalSize)}</span>
        {/if}
        <span class="spacer"></span>
        {#if !busy}<button class="btn" data-testid="new-files" onclick={clearAll}>Clear All</button>{/if}
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
            <EffortLadder effort={effort} routeMode={routeMode} onEffortInput={setEffortValue} />
            <CommandPreviewPanel previews={commandPreviews} error={commandPreviewError} />
          </div>
        </div>

        <div style="display:flex;flex-direction:column;gap:12px">
          <div class="card">
            <h3>Quality</h3>
            <div class="in">
              <QualitySliders distance={distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} locked={routeMode === 'lossless'} onDistance={setDistanceValue} onQuality={setQualityValue} />
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
              {#if toolchain?.flagsLocked}
                <div class="banner warn"><span class="ic">!</span><span>Expert flags are locked because the installed cjxl version differs from the generated help.</span></div>
              {/if}
              <div class="flag-actions">
                <button class="btn ghost" onclick={resetExpertFlags} disabled={Boolean(toolchain?.flagsLocked)}>Reset Expert flags</button>
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
          <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" onclick={() => void startConversion()} disabled={!canConvert}>Convert {pendingPaths.length || files.length} files</button>
        </div>
      </div>
    </div>
  {:else if view === 'automatic'}
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
                {@const succeeded = !result.error && !result.skipped && !result.cancelled}
                <div class="row" style="padding:5px 0">
                  <span class="k" style="font-family:var(--mono);color:var(--ink)">{result.input.split(/[\\/]/).pop()}</span>
                  {#if succeeded}
                    <span class="v mono-mini">{formatBytes(result.inputSize)} -&gt; {formatBytes(result.outputSize)}</span>
                    <span class="delta-chip" class:neg={savedPct(result.inputSize, result.outputSize) < 0}>{formatDelta(result.inputSize, result.outputSize)}</span>
                  {:else}
                    <span class="v">{result.error ? 'failed' : result.skipped ? 'skipped' : 'cancelled'}</span>
                  {/if}
                </div>
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
              <button class="btn" data-testid="check-updates" onclick={() => void refreshToolchain()} disabled={installing}>Check for updates</button>
              {#if toolchain?.updateAvailable || toolchain?.needsInstall}
                <button class="btn primary" style="background:var(--p-encode)" onclick={() => void installToolchain()} disabled={installing}>{toolchain?.needsInstall ? 'Install toolchain' : `Update to ${toolchain.latestVersion}`}</button>
              {/if}
            </div>
            {#if installing && installProgress}
              <div class="mini" style="margin-top:8px">{installProgress.phase === 'downloading' ? 'Downloading' : 'Installing'}{#if installProgress.phase === 'downloading' && installProgress.total > 0} — {formatBytes(installProgress.downloaded)} / {formatBytes(installProgress.total)}{/if}</div>
              {#if installProgress.phase === 'downloading' && installProgress.total > 0}
                <div class="bar" style="margin-top:4px"><i style={`width:${Math.min(100, (installProgress.downloaded / installProgress.total) * 100)}%`}></i></div>
              {/if}
            {/if}
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
          <div class="banner info" style="margin:10px 10px 0"><span class="ic">i</span><span>Rules are edited in the YAML file: select a preset below and use Open in Editor, then Reload. A JSON schema (<code>preset.schema.json</code>) validates the file in YAML-aware editors.</span></div>
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
            <button class="btn ghost" data-testid="preset-open-editor" onclick={() => void openPresetInEditor()} disabled={!selectedPreset}>Open in Editor</button>
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
                    <label for="preset-collision">On collision</label>
                    <select id="preset-collision" bind:value={presetCollisionDraft} disabled={selectedPresetReadOnly}>
                      <option value="skip">Skip</option>
                      <option value="number">Number the new file</option>
                      <option value="overwrite">Overwrite existing</option>
                    </select>
                    <button class="btn" data-testid="preset-save-output" onclick={() => void savePresetOutput()} disabled={selectedPresetReadOnly || !presetOutputDirty}>Save</button>
                  </div>
                  {#if selectedPresetReadOnly}
                    <div class="mini">This factory preset is read-only. Duplicate it to change the output settings.</div>
                  {:else if presetOutputDirty}
                    <div class="mini warning">Saving rewrites the YAML file and drops comments in it. Hand-edited files: better use Open in Editor.</div>
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
  {:else if view === 'history'}
    <div class="body">
      <div class="cols wide-right">
        <div class="card">
          <h3>History <span class="r">{historyEntries.length} conversions
            <button class="icon-btn" aria-label="Reload history" title="Reload" onclick={() => void loadHistory()}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8 8 0 1 0-.9 4"></path><path d="M20 4v6h-6"></path></svg></button>
            <button class="btn danger" style="padding:2px 10px" data-testid="history-clear" onclick={() => void clearHistoryAll()} disabled={historyEntries.length === 0}>Clear</button></span></h3>
          {#if !historyLoaded}
            <div class="empty">Loading history...</div>
          {:else if historyEntries.length === 0}
            <div class="empty">No conversions recorded yet. Successfully converted files appear here.</div>
          {:else}
            <table class="files group-files" data-testid="history-table">
              <colgroup>
                <col class="gf-file" />
                <col class="gf-hug" />
                <col class="gf-hug" />
                <col class="gf-hug" />
                <col class="gf-hug" />
              </colgroup>
              <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Original</th><th style="text-align:right">JXL</th><th style="text-align:right">Saved</th></tr></thead>
              <tbody>
                {#each historyEntries as entry (entry.at + entry.output)}
                  <tr
                    class="clickable"
                    class:selected={historyMeta.entry === entry}
                    onclick={() => { if (historyMeta.entry !== entry) void inspectHistoryEntry(entry); }}
                    title="{entry.at} · preset {entry.preset}"
                  >
                    <td class="fn" title={`${entry.at} · ${entry.input}`}>{entry.input.split(/[\\/]/).pop()}</td>
                    <td><span class={`badge ${routeClass(entry.route)}`}>{entry.route}</span></td>
                    <td class="num">{formatBytes(entry.inputSize)}</td>
                    <td class="num">{formatBytes(entry.outputSize)}</td>
                    <td class="num"><span class="delta-chip" class:neg={savedPct(entry.inputSize, entry.outputSize) < 0}>{formatDelta(entry.inputSize, entry.outputSize)}</span></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>
        <div style="display:flex;flex-direction:column;gap:12px">
          <JxlInfoPanel
            label={historyMeta.entry ? (historyMeta.error ? 'unavailable' : compactPath(historyMeta.entry.output, 34)) : 'select an entry'}
            emptyText="Select a history entry to inspect its JPEG XL metadata."
            hasSelection={historyMeta.entry != null}
            loading={historyMeta.loading}
            error={historyMeta.error}
            output={historyMeta.output}
          />
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
    {#if installing && installProgress}
      <span>{installProgress.phase === 'downloading' ? 'Downloading libjxl' : 'Installing libjxl'}{#if installProgress.phase === 'downloading' && installProgress.total > 0} — {formatBytes(installProgress.downloaded)} / {formatBytes(installProgress.total)}{/if}</span>
    {:else if busy || view === 'automatic'}
      <span>{progress.completed} done - {progress.failed} failed</span><span>{formatRate(progress.throughput)}</span>
    {:else if view === 'tools' && toolchain?.updateAvailable}
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
