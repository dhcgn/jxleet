<script lang="ts">
  import type { CommandPreview } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';

  interface Props {
    previews: CommandPreview[];
    error: string;
  }
  let { previews, error }: Props = $props();
</script>

{#if error}
  <div class="banner warn" data-testid="cmd-preview-error"><span class="ic">!</span><span>{error}</span></div>
{:else if previews.length === 0}
  <div class="cmd" data-testid="cmd-preview">Select a preset to preview the resolved cjxl command.</div>
{:else}
  <div class="command-previews" data-testid="cmd-preview">
    {#each previews as preview}
      <div class="command-preview">
        <div class="mini">{preview.matches?.join(', ') ?? 'default rule'}</div>
        <div class="cmd">{preview.command}</div>
      </div>
    {/each}
  </div>
{/if}
