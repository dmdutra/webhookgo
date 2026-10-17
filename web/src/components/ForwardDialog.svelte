<script lang="ts">
  import Info from '@lucide/svelte/icons/info';
  import { untrack } from 'svelte';
  import { defaultForward, forwardUrl, type ForwardSettings } from '../lib/forward';
  import { app } from '../lib/state.svelte';
  import Modal from './Modal.svelte';

  let { open, onclose }: { open: boolean; onclose: () => void } = $props();

  const METHODS = ['', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
  let s = $state<ForwardSettings>({ ...defaultForward });

  $effect(() => {
    if (open) s = untrack(() => ({ ...app.forwardSettings }));
  });

  const preview = $derived(app.selected && s.url ? forwardUrl(s, app.selected) : '');

  function save(andSend = false) {
    app.saveForward({ ...s, url: s.url.trim() });
    onclose();
    if (andSend && app.selectedId && s.url.trim()) app.forward(app.selectedId);
    else app.toast(s.auto && s.url ? 'Auto-forward is on' : 'Forwarding settings saved', 'success');
  }
</script>

<Modal {open} {onclose} title="Forwarding" subtitle="Replay captured requests to another URL, straight from this browser">
  <form id="forward-form" class="form" onsubmit={(e) => { e.preventDefault(); save(); }}>
    <div class="field">
      <label for="fwd-url">Target URL</label>
      <input id="fwd-url" class="input mono" type="url" placeholder="http://localhost:3000/webhooks" bind:value={s.url} />
      {#if preview}<span class="hint mono">→ {preview}</span>{/if}
    </div>

    <div class="row2">
      <div class="field">
        <label for="fwd-method">Method</label>
        <select id="fwd-method" class="input" bind:value={s.method}>
          {#each METHODS as m (m)}<option value={m}>{m || 'Same as the request'}</option>{/each}
        </select>
      </div>
      <div class="field">
        <label for="fwd-headers">Headers</label>
        <select id="fwd-headers" class="input" bind:value={s.headers}>
          <option value="all">Forward all headers</option>
          <option value="list">Only the ones listed</option>
          <option value="none">No headers</option>
        </select>
      </div>
    </div>
    {#if s.headers === 'list'}
      <div class="field">
        <label for="fwd-list">Header names</label>
        <input id="fwd-list" class="input mono" placeholder="content-type, x-hub-signature-256" bind:value={s.headerList} />
      </div>
    {/if}

    <label class="check">
      <input type="checkbox" class="switch" bind:checked={s.appendPath} />
      <span>Append the path and query string<small>…/{app.token?.uuid.slice(0, 8)}…/orders?id=1 → target/orders?id=1</small></span>
    </label>
    <label class="check">
      <input type="checkbox" class="switch" bind:checked={s.auto} />
      <span>Auto-forward new requests<small>While this tab stays open</small></span>
    </label>

    <p class="note">
      <Info size={15} />
      <span>
        Requests are sent by your browser, so <code>localhost</code> and private networks work. The target must allow
        CORS for this origin (<code>{location.origin}</code>); browsers also drop headers such as <code>Host</code> and
        <code>Cookie</code>.
      </span>
    </p>
  </form>

  {#snippet footer()}
    <span class="grow"></span>
    {#if app.selectedId}
      <button type="button" class="btn sm" disabled={!s.url.trim()} onclick={() => save(true)}>Save & forward selected</button>
    {/if}
    <button type="submit" form="forward-form" class="btn primary sm">Save</button>
  {/snippet}
</Modal>

<style>
  .form {
    display: grid;
    gap: 16px;
  }
  .row2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .check {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    cursor: pointer;
  }
  .check .switch {
    margin-top: 1px;
  }
  small {
    display: block;
    color: var(--text-3);
    font-size: 12px;
  }
  .note {
    display: flex;
    gap: 10px;
    padding: 10px 12px;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--text-2);
    font-size: 12.5px;
  }
  .note :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--accent-strong);
  }
  code {
    font-size: 11.5px;
  }
  .grow {
    flex: 1;
  }
  .hint {
    overflow-wrap: anywhere;
  }
</style>
