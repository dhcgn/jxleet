<script lang="ts">
  import type { FilePreview, ProgressUpdate } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { formatBytes, formatDelta, formatEta, formatRate, savedPct } from '../lib/format';

  interface Props {
    presetName: string;
    processes: number;
    threads: number;
    progress: ProgressUpdate;
    results: { input: string; error: string; skipped: boolean; cancelled: boolean; inputSize: number; outputSize: number }[];
    files: FilePreview[];
    onTogglePause(): void;
    onCancel(): void;
  }
  let { presetName, processes, threads, progress, results, files, onTogglePause, onCancel }: Props = $props();
</script>

<div class="body">
  <div class="toolbar" style="margin:-12px -12px 12px">
    <span class="badge b-reencode">Running</span>
    <span class="mini">{presetName || 'preset'} - {processes} processes - {threads} threads</span>
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
