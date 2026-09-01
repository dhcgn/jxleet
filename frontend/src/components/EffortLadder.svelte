<script lang="ts">
  import { Service } from '../../bindings/github.com/dhcgn/jxleet/internal/app';
  import { effortNames, effortTools, stageIndexAt } from '../lib/effort';
  import type { RouteMode } from '../lib/types';

  interface Props {
    effort: number;
    routeMode: RouteMode;
    onEffortInput: (value: number) => void;
  }
  let { effort, routeMode, onEffortInput }: Props = $props();
</script>

<div class="ladder-head">
  <span class="nm">{effortNames[effort - 1]}</span>
  <span class="hint">{effort === 7 ? 'cjxl default' : effort >= 9 ? 'noticeably slower' : effort <= 3 ? 'very fast, larger file' : ''}</span>
</div>
<div class="effort-slider">
  <input type="range" min="1" max="10" value={effort} oninput={(event) => onEffortInput(Number((event.currentTarget as HTMLInputElement).value))} data-testid="effort-range" aria-label="Effort" />
</div>
<div class="ticks">
  {#each effortNames as name, index}
    <span class:on={index + 1 === effort}>{index + 1}</span>
  {/each}
</div>
<table class="grid" data-testid="effort-grid">
  <thead><tr><th>Tool</th><th class="c" colspan="10" style="text-align:center">1 - 10</th><th class="c">L</th><th class="c">LL</th></tr></thead>
  <tbody>
    {#each effortTools as tool}
      {@const applicable = routeMode === 'lossy' ? tool.lossy : tool.lossless}
      {@const stages = (routeMode === 'lossy' ? tool.stagesLossy : tool.stagesLossless) ?? tool.stages ?? []}
      <tr class:na={!applicable}>
        <td class="nm" title={tool.tip}>{tool.name}</td>
        {#each effortNames as _, index}
          {@const level = index + 1}
          {@const stageIndex = stageIndexAt(stages, level)}
          {@const rank = tool.alwaysYellow ? 2 : stages.length - 1 - stageIndex}
          <td class:colhi={level === effort} class="cell" title={stageIndex >= 0 ? `${tool.name} (e${stages[stageIndex].from}+): ${stages[stageIndex].label}` : `${tool.name}: not active at e${level}`}>
            <span class={`dot${stageIndex >= 0 ? ` d${rank}` : ''}`} class:on={stageIndex >= 0} class:cur={applicable && level === effort}></span>
          </td>
        {/each}
        <td class="mode">
          <span class="capability-icon" class:capability-yes={tool.lossy} role="img" aria-label={tool.lossy ? 'Lossy mode supported' : 'Lossy mode not supported'} title={tool.lossy ? 'Lossy mode supported' : 'Lossy mode not supported'}>
            {#if tool.lossy}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8 3 3 7-7"></path></svg>{:else}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4 4 12"></path></svg>{/if}
          </span>
        </td>
        <td class="mode">
          <span class="capability-icon" class:capability-yes={tool.lossless} role="img" aria-label={tool.lossless ? 'Lossless mode supported' : 'Lossless mode not supported'} title={tool.lossless ? 'Lossless mode supported' : 'Lossless mode not supported'}>
            {#if tool.lossless}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8 3 3 7-7"></path></svg>{:else}<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4 4 12"></path></svg>{/if}
          </span>
        </td>
      </tr>
    {/each}
  </tbody>
</table>
<div class="mini ladder-doc">What each effort level enables: <a href="https://github.com/libjxl/libjxl/blob/main/doc/encode_effort.md" onclick={(e) => { e.preventDefault(); void Service.OpenURL('https://github.com/libjxl/libjxl/blob/main/doc/encode_effort.md'); }}>libjxl — encode effort</a></div>
