<script lang="ts">
  import type { Bindings, PresetSummary } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';

  interface Props {
    presets: PresetSummary[];
    selectedPreset: string;
    bindings: Bindings;
    selectedData: PresetSummary | null;
    readOnly: boolean;
    outputDirty: boolean;
    policyDraft?: string;
    collisionDraft?: string;
    onSelect(name: string): void;
    onCreate(): void;
    onDuplicate(): void;
    onRename(): void;
    onDelete(): void;
    onOpenInEditor(): void;
    onReload(): void;
    onOpenStorage(location: string): void;
    onSaveOutput(): void;
    onSetBinding(entryPoint: string, value: string): void;
  }
  let {
    presets,
    selectedPreset,
    bindings,
    selectedData,
    readOnly,
    outputDirty,
    policyDraft = $bindable(''),
    collisionDraft = $bindable('skip'),
    onSelect,
    onCreate,
    onDuplicate,
    onRename,
    onDelete,
    onOpenInEditor,
    onReload,
    onOpenStorage,
    onSaveOutput,
    onSetBinding,
  }: Props = $props();

  let selectedRules = $derived(selectedData?.rules ?? []);
</script>

<div class="body">
  <div class="cols wide-right">
    <div class="card">
      <h3>Preset library <span class="r">{presets.length} stored
        <button class="icon-btn" aria-label="Reload presets from folder" title="Reload from folder" data-testid="preset-refresh" onclick={() => void onReload()}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8 8 0 1 0-.9 4"></path><path d="M20 4v6h-6"></path></svg></button>
        <button class="icon-btn" aria-label="Open preset storage folder" title="Open in Explorer" onclick={() => void onOpenStorage('presets')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></span></h3>
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
              <tr class:selected={selectedPreset === preset.name} aria-selected={selectedPreset === preset.name} onclick={() => onSelect(preset.name)}>
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
        <button class="btn" data-testid="preset-new" onclick={() => void onCreate()}>New preset</button>
        <button class="btn ghost" onclick={() => void onDuplicate()} disabled={!selectedPreset}>Duplicate</button>
        <button class="btn ghost" onclick={() => void onRename()} disabled={!selectedPreset || readOnly}>Rename</button>
        <button class="btn ghost" data-testid="preset-open-editor" onclick={() => void onOpenInEditor()} disabled={!selectedPreset}>Open in Editor</button>
        <button class="btn danger" style="margin-left:auto" onclick={() => void onDelete()} disabled={!selectedPreset || readOnly}>Delete</button>
      </div>
      {#if selectedData}
        <div class="preset-details" data-testid="preset-details">
          <h3>{selectedData.name} <span class="r">{readOnly ? 'read-only' : 'details'}</span></h3>
          <div class="in">
            <div class="preset-policy-editor">
              <label for="preset-output-policy">Output policy</label>
              <div class="preset-policy-controls">
                <select id="preset-output-policy" bind:value={policyDraft} disabled={readOnly}>
                  <option value="alongside">Alongside</option>
                  <option value="subfolder">Into subfolder</option>
                  <option value="replace">Replace via recycle bin</option>
                </select>
                <label for="preset-collision">On collision</label>
                <select id="preset-collision" bind:value={collisionDraft} disabled={readOnly}>
                  <option value="skip">Skip</option>
                  <option value="number">Number the new file</option>
                  <option value="overwrite">Overwrite existing</option>
                </select>
                <button class="btn" data-testid="preset-save-output" onclick={() => void onSaveOutput()} disabled={readOnly || !outputDirty}>Save</button>
              </div>
              {#if readOnly}
                <div class="mini">This factory preset is read-only. Duplicate it to change the output settings.</div>
              {:else if outputDirty}
                <div class="mini warning">Saving rewrites the YAML file and drops comments in it. Hand-edited files: better use Open in Editor.</div>
              {/if}
            </div>
            <div class="rule-details">
              <div class="eyebrow">File rules</div>
              {#if selectedRules.length === 0}
                <div class="empty">No rules.</div>
              {:else}
                <table class="rule-table">
                  <thead><tr><th>File</th><th>Core</th><th>Effort</th><th>JPEG</th></tr></thead>
                  <tbody>
                    {#each selectedRules as rule}
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
            <select style="width:100%" data-testid="bind-gui" value={bindings.gui} onchange={(event) => void onSetBinding('gui', (event.currentTarget as HTMLSelectElement).value)}>
              <option value="">Select a preset</option>
              {#each presets as preset}<option value={preset.name}>{preset.name}</option>{/each}
            </select>
            <div class="mini" style="margin-top:4px">Used by the main window.</div>
          </div>
          <div style="padding:8px 0;border-top:1px solid var(--line-soft)">
            <div style="font-size:12px;color:var(--ink);margin-bottom:3px">File-path invocation</div>
            <select style="width:100%" data-testid="bind-cli" value={bindings.cli} onchange={(event) => void onSetBinding('cli', (event.currentTarget as HTMLSelectElement).value)}>
              <option value="">Select a preset</option>
              {#each presets as preset}<option value={preset.name}>{preset.name}</option>{/each}
            </select>
            <div class="mini" style="margin-top:4px">Used by Lightroom and CLI invocations without --preset.</div>
          </div>
          <div style="padding:8px 0 2px;border-top:1px solid var(--line-soft)">
            <div style="font-size:12px;color:var(--ink);margin-bottom:3px">Explorer context menu</div>
            <select style="width:100%" data-testid="bind-menu" value={bindings.contextMenu} onchange={(event) => void onSetBinding('contextmenu', (event.currentTarget as HTMLSelectElement).value)}>
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
