<script lang="ts">
  import type { HistoryEntry } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { compactPath, formatBytes, formatDelta, savedPct } from '../lib/format';
  import { routeClass } from '../lib/routes';
  import JxlInfoPanel from '../components/JxlInfoPanel.svelte';

  interface Props {
    entries: HistoryEntry[];
    loaded: boolean;
    meta: { entry: HistoryEntry | null; output: string; error: string; loading: boolean };
    onReload(): void;
    onClear(): void;
    onInspect(entry: HistoryEntry): void;
  }
  let { entries, loaded, meta, onReload, onClear, onInspect }: Props = $props();
</script>

<div class="body">
  <div class="cols wide-right">
    <div class="card">
      <h3>History <span class="r">{entries.length} conversions
        <button class="icon-btn" aria-label="Reload history" title="Reload" onclick={() => void onReload()}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 11a8 8 0 1 0-.9 4"></path><path d="M20 4v6h-6"></path></svg></button>
        <button class="btn danger" style="padding:2px 10px" data-testid="history-clear" onclick={() => void onClear()} disabled={entries.length === 0}>Clear</button></span></h3>
      {#if !loaded}
        <div class="empty">Loading history...</div>
      {:else if entries.length === 0}
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
            {#each entries as entry (entry.at + entry.output)}
              <tr
                class="clickable"
                class:selected={meta.entry === entry}
                onclick={() => { if (meta.entry !== entry) void onInspect(entry); }}
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
        label={meta.entry ? (meta.error ? 'unavailable' : compactPath(meta.entry.output, 34)) : 'select an entry'}
        emptyText="Select a history entry to inspect its JPEG XL metadata."
        hasSelection={meta.entry != null}
        loading={meta.loading}
        error={meta.error}
        output={meta.output}
      />
    </div>
  </div>
</div>
