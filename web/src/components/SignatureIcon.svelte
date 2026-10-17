<script lang="ts">
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import ShieldX from '@lucide/svelte/icons/shield-x';
  import type { SignatureStatus } from '../lib/types';

  let { status, size = 12 }: { status: SignatureStatus; size?: number } = $props();
</script>

{#if status === 'valid'}
  <ShieldCheck {size} />
{:else if status === 'invalid' || status === 'expired'}
  <ShieldX {size} />
{:else}
  <ShieldAlert {size} />
{/if}
