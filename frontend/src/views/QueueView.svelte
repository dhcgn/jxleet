<!-- jl:view.queue=Queue view: session-only staging with frozen per-item settings, global start/pause/cancel, per-process CPU/RAM while running, and successes auto-moving to History. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { formatBytes, formatDelta, formatDuration, savedPct } from '../lib/format';
  import { routeClass } from '../lib/routes';
  import type { QueueItem } from '../lib/types';

  interface Props {
    items: QueueItem[];
    queueRunning: boolean;
    paused: boolean;
    presetName: string;
    processes: number;
    threads: number;
    onStart(): void;
    onTogglePause(): void;
    onCancel(): void;
    onRemove(id: string): void;
    onReclaim(id: string): void;
    onShow(id: string): void;
    onCancelFile(id: string): void;
    onClearDone(): void;
  }
  let {
    items,
    queueRunning,
    paused,
    presetName,
    processes,
    threads,
    onStart,
    onTogglePause,
    onCancel,
    onRemove,
    onReclaim,
    onShow,
    onCancelFile,
    onClearDone,
  }: Props = $props();

  let openCount = $derived(items.filter((item) => item.status === 'waiting' || item.status === 'processing').length);
  let doneCount = $derived(items.filter((item) => item.status === 'done').length);
  let retryCount = $derived(items.filter((item) => item.status === 'failed' || item.status === 'cancelled' || item.status === 'skipped').length);
  let startable = $derived(items.filter((item) => item.status !== 'done' && item.status !== 'processing').length);
  let queueDone = $derived(items.filter((item) => item.status === 'done').length);
  let queueTotal = $derived(items.length);
  let queuePercent = $derived(queueTotal === 0 ? 0 : (queueDone / queueTotal) * 100);

  // Ticking clock for elapsed timers; only runs while the queue executes.
  let now = $state(Date.now());
  onMount(() => {
    const timer = setInterval(() => {
      if (queueRunning) now = Date.now();
    }, 1000);
    return () => clearInterval(timer);
  });

  // MM:ss once a task runs longer than 10s, else empty.
  function elapsedText(item: QueueItem): string {
    if (item.status !== 'processing' || !item.startedAt) return '';
    const sec = Math.floor((now - item.startedAt) / 1000);
    if (sec < 10) return '';
    return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
  }

  function resourcesText(item: QueueItem): string {
    if (item.status !== 'processing' || item.pid <= 0 || item.cpuTimeSeconds == null || item.memoryBytes == null) return '—';
    return `${item.cpuTimeSeconds.toFixed(1)}s CPU · ${formatBytes(item.memoryBytes)}`;
  }

  function statusText(item: QueueItem): string {
    switch (item.status) {
      case 'waiting':
        return 'waiting';
      case 'processing':
        return item.pid > 0 ? `PID ${item.pid}` : 'starting…';
      case 'done':
        return item.warning ? `done ⚠` : 'done';
      case 'failed':
        return item.error || 'failed';
      case 'cancelled':
        return 'cancelled';
      case 'skipped':
        return item.skipReason || 'skipped';
      default:
        return item.status;
    }
  }
</script>

<div class="body">
  <div class="run-strip" data-testid="queue-strip">
    <div class="run-head">
      <span class="badge b-reencode">{queueRunning ? (paused ? 'Paused' : 'Running') : 'Queue'}</span>
      <span class="run-count">{queueDone} of {queueTotal}{openCount > 0 ? ` · ${openCount} open` : ''}</span>
      <span class="mini">{presetName || 'preset'} - {processes} processes - {threads} threads</span>
      <span class="spacer"></span>
      {#if queueRunning}
        <button class="btn" onclick={onTogglePause}>{paused ? 'Resume' : 'Pause'}</button>
        <button class="btn danger" data-testid="cancel" onclick={onCancel}>Cancel</button>
      {:else}
        {#if doneCount > 0}
          <button class="btn ghost" onclick={onClearDone}>Clear done</button>
        {/if}
        <button class="btn primary convert-action" style="background:var(--p-encode);padding:11px" data-testid="start-queue" onclick={onStart} disabled={startable === 0}>
          {retryCount > 0 && queueDone > 0 ? `Retry ${retryCount} · Start ${startable}` : `Start ${startable} file${startable === 1 ? '' : 's'}`}
        </button>
      {/if}
    </div>
    <div class="bar"><i style={`width:${Math.min(100, queuePercent)}%`}></i></div>
  </div>

  <div class="card">
    <h3>Staged files <span class="r">{items.length} item{items.length === 1 ? '' : 's'} · session-only, closing discards them</span></h3>
    {#if items.length === 0}
      <div class="empty">Queue is empty. Add files in Main, tune the settings, then “Move to queue”.</div>
    {:else}
      <table class="files group-files" data-testid="queue-table">
        <colgroup>
          <col class="gf-file" />
          <col class="gf-hug" />
          <col class="gf-hug" />
          <col class="gf-hug" />
          <col class="gf-hug" />
          <col class="gf-status" />
        </colgroup>
        <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Original</th><th style="text-align:right">JXL</th><th style="text-align:right">Saved</th><th>Status</th></tr></thead>
        <tbody>
          {#each items as item (item.id)}
            <tr style="--custom-contextmenu: queue-row; --custom-contextmenu-data: {item.id}; --default-contextmenu: hide" title={item.path}>
              <td class="fn" title={item.path}>
                {item.name}
                <div class="mono-mini" title={item.flagsSet ? `${item.snapshot} + extra flags` : item.snapshot}>{item.snapshot}{#if item.flagsSet} +flags{/if}</div>
              </td>
              <td><span class={`badge ${routeClass(item.route)}`}>{item.route || 'pending'}</span></td>
              <td class="num">{formatBytes(item.size)}</td>
              <td class="num">{item.status === 'done' ? formatBytes(item.outputSize) : '-'}</td>
              <td class="num">
                {#if item.status === 'done'}
                  <span class="delta-chip" class:neg={savedPct(item.size, item.outputSize) < 0}>{formatDelta(item.size, item.outputSize)}</span>
                {:else}—{/if}
              </td>
              <td class="status-cell" class:success={item.status === 'done'} class:error={item.status === 'failed'} title={item.warning || item.error || undefined}>
                {statusText(item)}{item.warning ? ' ⚠' : ''}
                <div class="mono-mini">
                  {#if item.status === 'processing'}
                    {resourcesText(item)}
                    {#if elapsedText(item) !== ''} · {elapsedText(item)}{/if}
                  {:else if item.status === 'done'}
                    {formatDuration(item.durationSeconds)}
                  {/if}
                </div>
                <div style="display:flex;gap:6px;margin-top:4px;flex-wrap:wrap">
                  {#if item.status === 'processing'}
                    <button class="btn" onclick={() => onCancelFile(item.id)}>Cancel file</button>
                  {:else}
                    <button class="btn ghost" onclick={() => onReclaim(item.id)} title="Back to Main intake with this snapshot">Reclaim</button>
                    <button class="btn ghost" onclick={() => onShow(item.id)} title="Reveal in Explorer">Show</button>
                    <button class="btn ghost" onclick={() => onRemove(item.id)} title="Remove from queue">Remove</button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
      <div class="mini" style="padding:8px 2px 0">Right-click a row for the same actions. Failed, cancelled and skipped rows stay for retry; successes are recorded to History.</div>
    {/if}
  </div>
</div>
