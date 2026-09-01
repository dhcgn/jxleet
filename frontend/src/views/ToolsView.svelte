<script lang="ts">
  import type { Bindings, ToolchainProgress, ToolchainStatus } from '../../bindings/github.com/dhcgn/jxleet/internal/app/models';
  import { formatBytes } from '../lib/format';

  interface Props {
    tools: {
      status: ToolchainStatus | null;
      error: string;
      installing: boolean;
      progress: ToolchainProgress | null;
      contextMenu: boolean;
    };
    bindings: Bindings;
    onRefresh(): void;
    onInstall(): void;
    onRegister(): void;
    onUnregister(): void;
    onOpenStorage(location: string): void;
  }
  let { tools, bindings, onRefresh, onInstall, onRegister, onUnregister, onOpenStorage }: Props = $props();
</script>

<div class="body">
  {#if tools.error}
    <div class="banner warn"><span class="ic">!</span><span>{tools.error}</span><button class="btn ghost" style="margin-left:auto" onclick={() => void onRefresh()}>Retry</button></div>
  {/if}
  {#if tools.status?.flagsLocked}
    <div class="banner warn"><span class="ic">!</span><span><b>Expert flags are locked.</b> Installed libjxl {tools.status.flagToolVersion || 'unknown'} differs from generated flags {tools.status.flagBaseVersion}.</span></div>
  {/if}
  <div class="cols">
    <div class="card">
      <h3>libjxl</h3>
      <div class="in">
        <div class="row"><span class="k">Installed</span><span class="v">{tools.status?.installedVersion || 'not installed'}</span></div>
        <div class="row"><span class="k">cjxl</span><span class="v">{tools.status?.cjxlVersion || '-'}</span></div>
        <div class="row"><span class="k">djxl</span><span class="v">{tools.status?.djxlVersion || '-'}</span></div>
        <div class="row"><span class="k">jxlinfo</span><span class="v">{tools.status?.jxlinfoVersion || '-'}</span></div>
        <div class="row"><span class="k">Latest version</span><span class:warning={tools.status?.updateAvailable} class="v">{tools.status?.latestVersion || '-'}</span></div>
        <div class="row"><span class="k">Asset</span><span class="v">jxl-x64-windows-static.zip</span></div>
        <div style="display:flex;gap:8px;margin-top:10px;flex-wrap:wrap">
          <button class="btn" data-testid="check-updates" onclick={() => void onRefresh()} disabled={tools.installing}>Check for updates</button>
          {#if tools.status?.updateAvailable || tools.status?.needsInstall}
            <button class="btn primary" style="background:var(--p-encode)" onclick={() => void onInstall()} disabled={tools.installing}>{tools.status?.needsInstall ? 'Install toolchain' : `Update to ${tools.status.latestVersion}`}</button>
          {/if}
        </div>
        {#if tools.installing && tools.progress}
          <div class="mini" style="margin-top:8px">{tools.progress.phase === 'downloading' ? 'Downloading' : 'Installing'}{#if tools.progress.phase === 'downloading' && tools.progress.total > 0} — {formatBytes(tools.progress.downloaded)} / {formatBytes(tools.progress.total)}{/if}</div>
          {#if tools.progress.phase === 'downloading' && tools.progress.total > 0}
            <div class="bar" style="margin-top:4px"><i style={`width:${Math.min(100, (tools.progress.downloaded / tools.progress.total) * 100)}%`}></i></div>
          {/if}
        {/if}
        {#if tools.status && !tools.status.updateAvailable && !tools.status.needsInstall}
          <div class="mini" style="margin-top:6px">libjxl is up to date.</div>
        {/if}
      </div>
    </div>
    <div style="display:flex;flex-direction:column;gap:12px">
      <div class="card">
        <h3>Explorer context menu</h3>
        <div class="in">
          <div class="row"><span class="k">Status</span><span class:success={tools.contextMenu} class:muted={!tools.contextMenu} class="v">{tools.contextMenu ? 'registered' : 'not registered'}</span></div>
          <div class="row"><span class="k">Preset</span><span class="v">{bindings.contextMenu || 'not bound'}</span></div>
          <div class="mini" style="margin-top:8px">Per-user registration; Windows 11 shows it under Show more options.</div>
          <div style="display:flex;gap:8px;margin-top:10px;flex-wrap:wrap">
            <button class="btn" onclick={() => void onRegister()} disabled={!bindings.contextMenu}>Register</button>
            <button class="btn danger" onclick={() => void onUnregister()} disabled={!tools.contextMenu}>Remove entry</button>
          </div>
        </div>
      </div>
      <div class="card">
        <h3>Flag changes</h3>
        <div class="in kv">
          <div><span>Base</span><span>{tools.status?.flagBaseVersion || '-'}</span></div>
          <div><span>Installed</span><span>{tools.status?.flagToolVersion || '-'}</span></div>
          <div><span>Added</span><span>{tools.status?.addedFlags?.join(', ') || 'none'}</span></div>
          <div><span>Removed</span><span>{tools.status?.removedFlags?.join(', ') || 'none'}</span></div>
        </div>
      </div>
      <div class="card">
        <h3>Storage locations</h3>
        <div class="in kv">
          <div><span>Settings</span><span>%APPDATA%\\jxleet\\config.yaml</span><button class="icon-btn" aria-label="Open settings folder" title="Open in Explorer" onclick={() => void onOpenStorage('config')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
          <div><span>Presets</span><span>%APPDATA%\\jxleet\\presets\\</span><button class="icon-btn" aria-label="Open presets folder" title="Open in Explorer" onclick={() => void onOpenStorage('presets')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
          <div><span>Binaries</span><span>%LOCALAPPDATA%\\jxleet\\bin\\</span><button class="icon-btn" aria-label="Open binaries folder" title="Open in Explorer" onclick={() => void onOpenStorage('bin')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
          <div><span>Logs</span><span>%LOCALAPPDATA%\\jxleet\\logs\\</span><button class="icon-btn" aria-label="Open logs folder" title="Open in Explorer" onclick={() => void onOpenStorage('logs')}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H10l2 2h7.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"></path><path d="M3 9h18"></path></svg></button></div>
        </div>
      </div>
    </div>
  </div>
</div>
