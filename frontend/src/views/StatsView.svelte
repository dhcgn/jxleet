<script lang="ts">
  import { formatBytes, formatDelta, savedPct } from '../lib/format';
  import { routeClass } from '../lib/routes';

  // Mocked Stats view: static data only, no backend/toolchain needed, so it
  // can be iterated with `wails3 dev`. Clearly marked as mock until wired to
  // real run data. Kitchen-sink state: every element populated.

  interface MockRow {
    name: string;
    route: string;
    inSize: number;
    outSize: number;
    preset: string;
    at: string;
  }

  const summary = { files: 128, bytesIn: 4527833600, bytesOut: 3115827200, avgSec: 1.8 };
  const byRoute = [
    { route: 'Transcode', count: 64, inSize: 2411724800, outSize: 1932735283 },
    { route: 'Reencode', count: 41, inSize: 1493172224, outSize: 805306368 },
    { route: 'Encode', count: 23, inSize: 622936576, outSize: 377487360 },
  ];
  const byDistance = [
    { label: 'd 0.0', count: 64 },
    { label: 'd 0.5', count: 12 },
    { label: 'd 1.0', count: 33 },
    { label: 'd 1.5', count: 14 },
    { label: 'd 2.0+', count: 5 },
  ];
  const maxRoute = Math.max(...byRoute.map((r) => r.count));
  const maxDist = Math.max(...byDistance.map((r) => r.count));
  const rows: MockRow[] = [
    { name: 'DSC_0001.jxl', route: 'Transcode', inSize: 12582912, outSize: 10066330, preset: 'archive-lossless', at: '2026-09-11 10:41:02' },
    { name: 'DSC_0002.jxl', route: 'Transcode', inSize: 11534336, outSize: 9227469, preset: 'archive-lossless', at: '2026-09-11 10:40:58' },
    { name: 'pano.jxl', route: 'Reencode', inSize: 36700160, outSize: 19922944, preset: 'web-d15-e7', at: '2026-09-11 10:39:44' },
    { name: 'logo.jxl', route: 'Encode', inSize: 524288, outSize: 312475, preset: 'archive-lossless', at: '2026-09-11 10:38:12' },
    { name: 'scan-01.jxl', route: 'Encode', inSize: 8304721, outSize: 5050375, preset: 'archive-lossless', at: '2026-09-11 10:37:55' },
    { name: 'wedding-214.jxl', route: 'Reencode', inSize: 20971520, outSize: 11534336, preset: 'web-d15-e7', at: '2026-09-11 10:36:31' },
    { name: 'wedding-215.jxl', route: 'Reencode', inSize: 22020096, outSize: 12058624, preset: 'web-d15-e7', at: '2026-09-11 10:36:28' },
    { name: 'icon-set.jxl', route: 'Encode', inSize: 1048576, outSize: 706543, preset: 'archive-lossless', at: '2026-09-11 10:35:09' },
  ];
</script>

<div class="body">
  <div class="banner info" role="status"><span class="ic">i</span><span><b>Mock data.</b> This Stats view is a static placeholder for iterating on layout — not wired to real run data yet.</span></div>
  <div class="cols wide-right">
    <div style="display:flex;flex-direction:column;gap:12px">
      <div class="card">
        <h3>Totals <span class="r">mock</span></h3>
        <div class="in kv">
          <div><span>Converted</span><span>{summary.files} files</span></div>
          <div><span>Bytes</span><span>{formatBytes(summary.bytesIn)} -&gt; {formatBytes(summary.bytesOut)}</span></div>
          <div><span>Saved</span><span class="success">{formatDelta(summary.bytesIn, summary.bytesOut)}</span></div>
          <div><span>Avg per file</span><span>{summary.avgSec.toFixed(1)} s</span></div>
        </div>
      </div>

      <div class="card">
        <h3>By route <span class="r">mock</span></h3>
        <div class="in">
          {#each byRoute as r (r.route)}
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span class={`badge ${routeClass(r.route)}`}>{r.route}</span>
              <div class="bar" style="flex:1"><i style={`width:${Math.round((r.count / maxRoute) * 100)}%`}></i></div>
              <span class="mono-mini">{r.count} files · {formatDelta(r.inSize, r.outSize)}</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="card">
        <h3>By distance <span class="r">mock</span></h3>
        <div class="in">
          {#each byDistance as d (d.label)}
            <div style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
              <span class="mono-mini" style="min-width:44px">{d.label}</span>
              <div class="bar" style="flex:1"><i style={`width:${Math.round((d.count / maxDist) * 100)}%`}></i></div>
              <span class="mono-mini">{d.count}</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="card">
        <h3>Recent runs <span class="r">{rows.length} shown · mock</span></h3>
        <table class="files group-files" data-testid="stats-table">
          <colgroup>
            <col class="gf-file" />
            <col class="gf-hug" />
            <col class="gf-hug" />
            <col class="gf-hug" />
          </colgroup>
          <thead><tr><th>File</th><th>Route</th><th style="text-align:right">Original</th><th style="text-align:right">Saved</th></tr></thead>
          <tbody>
            {#each rows as row (row.name)}
              <tr title={`${row.at} · preset ${row.preset}`}>
                <td class="fn" title={row.name}>{row.name}</td>
                <td><span class={`badge ${routeClass(row.route)}`}>{row.route}</span></td>
                <td class="num">{formatBytes(row.inSize)}</td>
                <td class="num"><span class="delta-chip" class:neg={savedPct(row.inSize, row.outSize) < 0}>{formatDelta(row.inSize, row.outSize)}</span></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <div style="display:flex;flex-direction:column;gap:12px">
      <div class="card">
        <h3>Presets <span class="r">mock</span></h3>
        <div class="in kv">
          <div><span>archive-lossless</span><span>87 files</span></div>
          <div><span>web-d15-e7</span><span>41 files</span></div>
        </div>
      </div>
      <div class="card">
        <h3>Largest saving <span class="r">mock</span></h3>
        <div class="in kv">
          <div><span>pano.jxl</span><span class="success">{formatDelta(36700160, 19922944)}</span></div>
          <div><span>wedding-214.jxl</span><span class="success">{formatDelta(20971520, 11534336)}</span></div>
        </div>
      </div>
    </div>
  </div>
</div>
