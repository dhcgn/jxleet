<!-- jl:view.automatic=Running view: the compact window of an external CLI/Explorer run, with progress, every file's live status (failures with their reason) and a result-row menu to open Explorer or the converted file. -->
<script lang="ts">
  import type { FileUpdate, ProgressUpdate } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { formatBytes, formatDelta, formatEta, formatRate, savedPct } from '../lib/format';
  import { resultMenuStyle } from '../lib/menu';

  interface Props {
    presetName: string;
    processes: number;
    threads: number;
    progress: ProgressUpdate;
    results: FileUpdate[];
    inputs: string[]; // every input the engine accepted, in arrival order
    running: string[]; // inputs whose encode has started and not yet finished
    onTogglePause(): void;
    onCancel(): void;
  }
  let { presetName, processes, threads, progress, results, inputs, running, onTogglePause, onCancel }: Props = $props();

  type RowStatus = 'waiting' | 'running' | 'done' | 'failed' | 'skipped' | 'cancelled';
  interface Row {
    key: string;
    name: string;
    path: string;
    status: RowStatus;
    result: FileUpdate | null;
  }

  const baseName = (path: string): string => path.split(/[\\/]/).pop() ?? path;
  const keyOf = (path: string): string => path.toLowerCase();
  const resultStatus = (result: FileUpdate): RowStatus =>
    result.error ? 'failed' : result.skipped ? 'skipped' : result.cancelled ? 'cancelled' : 'done';

  // One row per input of the run, in arrival order, with its live status. A
  // result whose input was missed (events from before this window subscribed)
  // still gets a row, so a failure is always visible and never just a counter.
  let rows = $derived.by(() => {
    const latest = new Map<string, FileUpdate>();
    for (const result of results) latest.set(keyOf(result.input), result);
    const active = new Set(running.map(keyOf));
    const seen = new Set<string>();
    const list: Row[] = [];
    const add = (path: string): void => {
      const key = keyOf(path);
      if (seen.has(key)) return;
      seen.add(key);
      const result = latest.get(key) ?? null;
      list.push({ key, name: baseName(path), path, result, status: result ? resultStatus(result) : active.has(key) ? 'running' : 'waiting' });
    };
    for (const path of inputs) add(path);
    for (const result of latest.values()) add(result.input);
    // Failures first (stable sort keeps arrival order otherwise): in a list of
    // hundreds a failed file must not hide below the fold.
    return list.sort((a, b) => Number(b.status === 'failed') - Number(a.status === 'failed'));
  });

  const menuStyle = (result: FileUpdate | null): string | undefined => (result ? resultMenuStyle(`r${result.seq}`) : undefined);
</script>

<div class="body">
  <div class="toolbar" style="margin:-12px -12px 12px">
    <span class="badge b-reencode">Running</span>
    <span class="mini">{presetName || 'preset'} - {processes} processes - {threads > 0 ? `${threads} threads` : 'auto threads'}</span>
    <span class="spacer"></span>
    <button class="btn" onclick={() => void onTogglePause()}>{progress.paused ? 'Resume' : 'Pause'}</button>
    <button class="btn danger" data-testid="cancel" onclick={() => void onCancel()}>Cancel</button>
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
          {#each results.slice(-4).reverse() as result (result.seq)}
            {@const status = resultStatus(result)}
            <div class="row" style={`padding:5px 0;flex-wrap:wrap;${menuStyle(result)}`}>
              <span class="k" style="font-family:var(--mono);color:var(--ink)" title={result.input}>{baseName(result.input)}</span>
              {#if status === 'done'}
                <span class="v mono-mini">{formatBytes(result.inputSize)} -&gt; {formatBytes(result.outputSize)}</span>
                <span class="delta-chip" class:neg={savedPct(result.inputSize, result.outputSize) < 0}>{formatDelta(result.inputSize, result.outputSize)}</span>
              {:else}
                <span class="v" class:error={status === 'failed'}>{status}</span>
                {#if result.error}<div class="qerr">{result.error}</div>{/if}
              {/if}
            </div>
          {/each}
        </div>
      </div>
    </div>
    <div class="card">
      <h3>Queue <span class="r">{#if progress.failed > 0}<span class="error">{progress.failed} failed</span> ·{/if} {Math.max(0, progress.total - progress.completed - progress.failed - progress.skipped)} open</span></h3>
      <div class="in q q-scroll" data-testid="queue">
        {#each rows as row (row.key)}
          <div class="qrow" title={row.path} style={menuStyle(row.result)}>
            {row.name} <span class={`st ${row.status}`}>{row.status}</span>
            {#if row.result?.error}<div class="qerr" title={row.result.error}>{row.result.error}</div>{/if}
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>
