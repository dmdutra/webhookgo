<script lang="ts">
  import CopyButton from './CopyButton.svelte';
  import type { KV } from '../lib/types';

  let { rows, empty = 'None' }: { rows: KV[]; empty?: string } = $props();
</script>

{#if rows.length}
  <div class="table" role="table">
    {#each rows as row, i (i)}
      <div class="row" role="row">
        <div class="name mono" role="cell">{row.name}</div>
        <div class="value mono" role="cell">
          {#if row.value === ''}<span class="muted">(empty)</span>{:else}{row.value}{/if}
        </div>
        <div class="copy"><CopyButton text={row.value} title="Copy value" /></div>
      </div>
    {/each}
  </div>
{:else}
  <p class="empty">{empty}</p>
{/if}

<style>
  .table {
    display: grid;
    grid-template-columns: minmax(110px, max-content) minmax(0, 1fr) auto;
    font-size: 12.5px;
  }
  .row {
    display: contents;
  }
  .row > div {
    padding: 6px 8px;
    border-top: 1px solid var(--border);
    min-width: 0;
  }
  .row:first-child > div {
    border-top: 0;
  }
  .row:hover > div {
    background: var(--bg-hover);
  }
  .name {
    max-width: 260px;
    color: var(--text-2);
    font-weight: 500;
    overflow-wrap: anywhere;
  }
  .value {
    color: var(--text);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .copy {
    padding: 3px 4px !important;
    opacity: 0;
    transition: opacity 0.1s;
  }
  .row:hover .copy,
  .copy:focus-within {
    opacity: 1;
  }
  @media (hover: none) {
    .copy {
      opacity: 1;
    }
  }
  .empty {
    color: var(--text-3);
    font-size: 13px;
  }
</style>
