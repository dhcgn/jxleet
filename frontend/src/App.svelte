<script lang="ts">
  import { onMount } from 'svelte';
  import { Service } from '../bindings/github.com/dhcgn/jxleet/internal/app';

  let ready = $state(false);
  let unbound = $state<string[]>([]);
  let loaded = $state(false);

  onMount(async () => {
    try {
      const status = await Service.GetStatus();
      ready = status.ready;
      unbound = status.unboundEntryPoints ?? [];
    } catch (e) {
      console.error(e);
    } finally {
      loaded = true;
    }
  });

  const entryLabel: Record<string, string> = {
    gui: 'Window',
    cli: 'Command line',
    contextmenu: 'Explorer context menu',
  };
</script>

<main class="app">
  <header class="topbar">
    <h1>jxleet</h1>
    <span class="tagline">JPEG-XL-Expert-Encoding-Tool</span>
  </header>

  {#if !loaded}
    <p class="muted">Loading…</p>
  {:else if ready}
    <section class="card">
      <p>Ready. Drop files or folders to begin.</p>
    </section>
  {:else}
    <section class="card warn">
      <h2>Setup needed</h2>
      <p>Each entry point needs a preset before jxleet will run:</p>
      <ul>
        {#each unbound as ep}
          <li>{entryLabel[ep] ?? ep}</li>
        {/each}
      </ul>
    </section>
  {/if}
</main>

<style>
  :global(body) {
    margin: 0;
    background: #06070f;
    color: #e5e7eb;
    font-family: 'Inter', system-ui, sans-serif;
  }
  .app {
    padding: 24px;
    min-width: 420px;
    box-sizing: border-box;
  }
  .topbar {
    display: flex;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 20px;
  }
  .topbar h1 {
    margin: 0;
    font-size: 22px;
  }
  .tagline {
    color: #9ca3af;
    font-size: 13px;
  }
  .card {
    background: #10131f;
    border: 1px solid #1f2433;
    border-radius: 10px;
    padding: 16px 18px;
  }
  .card.warn {
    border-color: #f97316;
  }
  .card h2 {
    margin: 0 0 8px;
    font-size: 16px;
  }
  .muted {
    color: #9ca3af;
  }
  ul {
    margin: 8px 0 0;
    padding-left: 18px;
  }
</style>
