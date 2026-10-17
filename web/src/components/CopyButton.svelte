<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import Copy from '@lucide/svelte/icons/copy';
  import { copyText } from '../lib/clipboard';
  import { app } from '../lib/state.svelte';

  interface Props {
    text: string | (() => string);
    label?: string;
    title?: string;
    class?: string;
    toast?: string;
  }
  let { text, label, title = 'Copy', class: cls = 'btn ghost icon xs', toast }: Props = $props();
  let done = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function copy(e: MouseEvent) {
    e.stopPropagation();
    const ok = await copyText(typeof text === 'function' ? text() : text);
    if (!ok) {
      app.toast('Copy failed — select the text and copy it manually', 'error');
      return;
    }
    if (toast) app.toast(toast, 'success', 1800);
    done = true;
    clearTimeout(timer);
    timer = setTimeout(() => (done = false), 1400);
  }
</script>

<button type="button" class={cls} class:copied={done} {title} aria-label={title} onclick={copy}>
  {#if done}<Check />{:else}<Copy />{/if}
  {#if label}<span>{done ? 'Copied' : label}</span>{/if}
</button>

<style>
  .copied {
    color: var(--success) !important;
  }
</style>
