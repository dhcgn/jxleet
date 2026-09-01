<script lang="ts">
  interface Props {
    label: string;
    emptyText: string;
    hasSelection: boolean;
    loading: boolean;
    error: string;
    output: string;
  }
  let { label, emptyText, hasSelection, loading, error, output }: Props = $props();
</script>

<div class="card">
  <h3>jxlinfo <span class="r">{label}</span></h3>
  <div class="in">
    {#if !hasSelection}
      <div class="empty">{emptyText}</div>
    {:else if loading}
      <div class="empty">Reading jxlinfo metadata...</div>
    {:else if error}
      <div class="banner warn" style="margin:0"><span class="ic">!</span><span>{error}</span></div>
    {:else}
      <pre class="metadata-output">{output}</pre>
    {/if}
  </div>
</div>
