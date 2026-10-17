<script lang="ts">
  import ChevronsDownUp from '@lucide/svelte/icons/chevrons-down-up';
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';
  import Download from '@lucide/svelte/icons/download';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import WrapText from '@lucide/svelte/icons/wrap-text';
  import { api } from '../lib/api';
  import { bodyBytes, detectKind, HEX_LIMIT, hexdump, jsonHtml, jsonText, parseJson, prettyXml, type BodyKind } from '../lib/body';
  import { bytes } from '../lib/format';
  import { app } from '../lib/state.svelte';
  import type { FullRequest } from '../lib/types';
  import CopyButton from './CopyButton.svelte';
  import JsonNode from './JsonNode.svelte';
  import KVTable from './KVTable.svelte';

  let { req }: { req: FullRequest } = $props();

  type Mode = 'tree' | 'pretty' | 'raw' | 'table' | 'hex' | 'base64' | 'preview';
  const MODES: Record<BodyKind, Mode[]> = {
    empty: [],
    json: ['tree', 'pretty', 'raw'],
    xml: ['pretty', 'raw'],
    form: ['table', 'raw'],
    text: ['raw', 'hex'],
    binary: ['hex', 'base64'],
    image: ['preview', 'hex', 'base64'],
  };
  const LABEL: Record<Mode, string> = {
    tree: 'Tree', pretty: 'Pretty', raw: 'Raw', table: 'Table', hex: 'Hex', base64: 'Base64', preview: 'Preview',
  };
  const BIG = 256 * 1024;

  const kind = $derived(detectKind(req));
  const modes = $derived(MODES[kind]);
  const big = $derived(req.content.length > BIG);
  const preferred = $derived.by((): Mode | undefined => {
    if (!app.prefs.pretty && modes.includes('raw')) return 'raw';
    if (kind === 'json' && big) return 'pretty';
    return modes[0];
  });

  // A manual choice sticks while browsing requests of the same kind.
  let chosen = $state<Partial<Record<BodyKind, Mode>>>({});
  const mode = $derived(chosen[kind] && modes.includes(chosen[kind]!) ? chosen[kind]! : preferred);

  let openDepth = $state(3);
  let treeKey = $state(0);

  const json = $derived(kind === 'json' ? parseJson(req.content) : null);
  const formRows = $derived(kind === 'form' ? req.form : []);
  const html = $derived.by(() => {
    if (mode === 'pretty' && json) return jsonHtml(json);
    if (mode === 'pretty' && kind === 'xml') return prettyXml(req.content) ?? '';
    return '';
  });
  const text = $derived.by(() => {
    if (mode === 'raw' || mode === 'base64') return req.content;
    if (mode === 'hex') return hexdump(bodyBytes(req));
    return '';
  });
  const copyValue = () => (mode === 'pretty' && json ? jsonText(json) : mode === 'hex' ? text : req.content);
  const imageSrc = $derived(kind === 'image' ? `data:${req.content_type.split(';')[0]};base64,${req.content}` : '');

  function expand(depth: number) {
    openDepth = depth;
    treeKey++;
  }
</script>

{#if kind === 'empty'}
  <p class="empty">No body{req.method === 'GET' || req.method === 'HEAD' ? '' : ' was sent'}.</p>
{:else}
  <div class="toolbar">
    {#if modes.length > 1}
      <div class="seg" role="tablist" aria-label="Body view">
        {#each modes as m (m)}
          <button type="button" role="tab" aria-selected={mode === m} class:on={mode === m} onclick={() => (chosen[kind] = m)}>{LABEL[m]}</button>
        {/each}
      </div>
    {/if}
    <span class="chip mono" title="Content-Type">{req.content_type || 'no content-type'}</span>
    <span class="grow"></span>
    {#if mode === 'tree'}
      <button type="button" class="btn ghost icon sm" title="Expand all" aria-label="Expand all" onclick={() => expand(Infinity)}><ChevronsUpDown /></button>
      <button type="button" class="btn ghost icon sm" title="Collapse all" aria-label="Collapse all" onclick={() => expand(1)}><ChevronsDownUp /></button>
    {/if}
    {#if mode !== 'tree' && mode !== 'table' && mode !== 'preview' && mode !== 'hex'}
      <button
        type="button"
        class="btn ghost icon sm"
        class:on={app.prefs.wrap}
        title="Wrap long lines"
        aria-label="Wrap long lines"
        aria-pressed={app.prefs.wrap}
        onclick={() => {
          app.prefs.wrap = !app.prefs.wrap;
          app.savePrefs();
        }}><WrapText /></button
      >
    {/if}
    <CopyButton text={copyValue} class="btn ghost icon sm" title="Copy body" toast="Body copied" />
    <a class="btn ghost icon sm" href={api.rawUrl(req.token_id, req.uuid, true)} title="Download body" aria-label="Download body" download><Download /></a>
  </div>

  {#if req.truncated}
    <div class="warn">
      <TriangleAlert size={15} />
      Body truncated: showing the first {bytes(req.content.length)} of {bytes(req.size)} (MAX_BODY_SIZE).
    </div>
  {/if}
  {#if mode === 'hex' && bodyBytes(req).length > HEX_LIMIT}
    <div class="warn"><TriangleAlert size={15} /> Showing the first {bytes(HEX_LIMIT)} only — download the body for the rest.</div>
  {/if}

  <div class="content">
    {#if mode === 'tree' && json}
      {#key treeKey}
        <div class="code tree"><JsonNode node={json} {openDepth} /></div>
      {/key}
    {:else if mode === 'pretty'}
      <!-- eslint-disable-next-line svelte/no-at-html-tags -- built from escaped text in lib/body.ts -->
      <pre class="code" class:wrap={app.prefs.wrap}>{@html html}</pre>
    {:else if mode === 'table'}
      <div class="table"><KVTable rows={formRows} /></div>
    {:else if mode === 'preview'}
      <div class="preview"><img src={imageSrc} alt="Request body" /></div>
    {:else}
      <pre class="code" class:wrap={app.prefs.wrap && mode !== 'hex'}>{text}</pre>
    {/if}
  </div>
{/if}

<style>
  .empty {
    padding: 12px 16px 14px;
    color: var(--text-3);
    font-size: 13px;
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
    border-bottom: 1px solid var(--border);
  }
  .grow {
    flex: 1;
  }
  .chip {
    max-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  @media (max-width: 640px) {
    .chip {
      display: none;
    }
  }
  .warn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: var(--warning-soft);
    color: var(--warning);
    font-size: 12.5px;
  }
  .content {
    background: var(--bg-sunken);
    border-radius: 0 0 var(--radius) var(--radius);
  }
  .code {
    max-height: none;
  }
  .tree {
    white-space: normal;
  }
  .table {
    padding: 6px 8px;
  }
  .preview {
    display: grid;
    place-items: center;
    padding: 16px;
    background: repeating-conic-gradient(var(--bg-hover) 0 25%, transparent 0 50%) 50% / 16px 16px;
  }
  .preview img {
    max-width: 100%;
    max-height: 480px;
    border-radius: 6px;
  }
</style>
