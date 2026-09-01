<script lang="ts">
  import type { CommandPreview, FlagInfo, FlagOverride } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { flagLabel, hiddenExpertFlags, isLinkedFlagKey } from '../lib/flags';
  import { formatDistance } from '../lib/quality';
  import type { RouteMode } from '../lib/types';
  import EffortLadder from '../components/EffortLadder.svelte';
  import QualitySliders from '../components/QualitySliders.svelte';
  import CommandPreviewPanel from '../components/CommandPreview.svelte';

  interface Props {
    routeMode: RouteMode;
    presetName: string;
    settings: { distance: number; effort: number; jpegLossless: boolean; threads: number };
    quality: number;
    outOfRange: boolean;
    previews: CommandPreview[];
    previewError: string;
    flagDefinitions: FlagInfo[];
    expertOverrides: FlagOverride[];
    flagsLocked: boolean;
    canConvert: boolean;
    pendingCount: number;
    filesCount: number;
    onSetRouteMode(mode: RouteMode): void;
    onSetEffort(value: number): void;
    onSetDistance(value: number): void;
    onSetQuality(quality: number): void;
    onResetFlags(): void;
    onSetFlagValue(key: string, value: string): void;
    onSetFlagEnabled(key: string, enabled: boolean): void;
    onStart(): void;
  }
  let {
    routeMode,
    presetName,
    settings,
    quality,
    outOfRange,
    previews,
    previewError,
    flagDefinitions,
    expertOverrides,
    flagsLocked,
    canConvert,
    pendingCount,
    filesCount,
    onSetRouteMode,
    onSetEffort,
    onSetDistance,
    onSetQuality,
    onResetFlags,
    onSetFlagValue,
    onSetFlagEnabled,
    onStart,
  }: Props = $props();

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

  function expertFlagValue(key: string): string {
    return expertOverrides.find((override) => override.key === key)?.value ?? '';
  }

  function expertFlagEnabled(key: string): boolean {
    return expertOverrides.some((override) => override.key === key && override.valueless);
  }
</script>

<div class="body">
  <div class="toolbar" style="margin:-12px -12px 12px">
    <div class="field"><span class="field-label">Route</span>
      <div class="seg">
        <button aria-pressed={routeMode === 'lossy'} onclick={() => onSetRouteMode('lossy')}>Lossy</button>
        <button aria-pressed={routeMode === 'lossless'} onclick={() => onSetRouteMode('lossless')}>Lossless</button>
      </div>
    </div>
    <span class="spacer"></span>
    <span class="mini">Editing preset: {presetName || 'none'}</span>
  </div>
  <div class="cols wide-right">
    <div class="card">
      <h3>Effort <span class="r">what this level adds</span></h3>
      <div class="in">
        <EffortLadder effort={settings.effort} routeMode={routeMode} onEffortInput={onSetEffort} />
        <CommandPreviewPanel {previews} error={previewError} />
      </div>
    </div>

    <div style="display:flex;flex-direction:column;gap:12px">
      <div class="card">
        <h3>Quality</h3>
        <div class="in">
          <QualitySliders distance={settings.distance} quality={quality} outOfRange={outOfRange} routeMode={routeMode} locked={routeMode === 'lossless'} onDistance={onSetDistance} onQuality={onSetQuality} />
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
          {#if flagsLocked}
            <div class="banner warn"><span class="ic">!</span><span>Expert flags are locked because the installed cjxl version differs from the generated help.</span></div>
          {/if}
          <div class="flag-actions">
            <button class="btn ghost" onclick={onResetFlags} disabled={flagsLocked}>Reset Expert flags</button>
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
                      disabled={linked || flagsLocked}
                      oninput={(event) => onSetFlagValue(flag.key, (event.currentTarget as HTMLInputElement).value)}
                    />
                  {:else}
                    <input
                      class="flag-toggle"
                      type="checkbox"
                      checked={expertFlagEnabled(flag.key)}
                      aria-label={flagLabel(flag)}
                      title={flag.description}
                      disabled={flagsLocked}
                      onchange={(event) => onSetFlagEnabled(flag.key, (event.currentTarget as HTMLInputElement).checked)}
                    />
                  {/if}
                </div>
              {/each}
            </div>
          {/each}
        </div>
      </div>
      <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" onclick={onStart} disabled={!canConvert}>Convert {pendingCount || filesCount} files</button>
    </div>
  </div>
</div>
