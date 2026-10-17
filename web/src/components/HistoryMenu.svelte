<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import History from '@lucide/svelte/icons/history';
  import X from '@lucide/svelte/icons/x';
  import { ago, shortId } from '../lib/format';
  import { app } from '../lib/state.svelte';
  import Dropdown from './Dropdown.svelte';
</script>

<Dropdown class="btn ghost icon sm" title="Your URLs" width={320}>
  {#snippet button()}<History />{/snippet}
  {#snippet children(close)}
    <div class="menu-label">Your URLs</div>
    {#each app.history as h (h.id)}
      {@const current = h.id === app.token?.uuid}
      <div class="row">
        <button
          type="button"
          class="menu-item"
          onclick={() => {
            close();
            if (!current) app.go(h.id);
          }}
        >
          <span class="grow">
            <span class="title">{h.description || shortId(h.id)}</span>
            <span class="sub mono">{h.id}</span>
            <span class="sub">created {ago(h.created_at, app.now)}</span>
          </span>
          {#if current}<Check />{/if}
        </button>
        {#if !current}
          <button type="button" class="btn ghost icon xs remove" title="Remove from history" aria-label="Remove from history" onclick={() => app.forget(h.id)}><X /></button>
        {/if}
      </div>
    {:else}
      <p class="none">No URLs yet.</p>
    {/each}
    {#if app.history.length > 1}
      <div class="menu-sep"></div>
      <button type="button" class="menu-item" onclick={() => app.clearHistory()}><X /> Clear other URLs from this browser</button>
    {/if}
  {/snippet}
</Dropdown>

<style>
  .row {
    position: relative;
  }
  .grow {
    display: grid;
    gap: 1px;
  }
  .title {
    font-weight: 600;
  }
  .sub {
    overflow: hidden;
    color: var(--text-3);
    font-size: 11.5px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub.mono {
    font-size: 11px;
  }
  .remove {
    position: absolute;
    top: 8px;
    right: 6px;
    opacity: 0;
  }
  .row:hover .remove,
  .remove:focus-visible {
    opacity: 1;
  }
  .none {
    padding: 8px 10px;
    color: var(--text-3);
    font-size: 13px;
  }
</style>
