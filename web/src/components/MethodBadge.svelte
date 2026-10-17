<script lang="ts">
  import { methodColor } from '../lib/format';

  let { method, size = 'md' }: { method: string; size?: 'md' | 'lg' } = $props();
</script>

<span class="method" class:lg={size === 'lg'} style:--mc={methodColor(method)}>{method}</span>

<style>
  .lg {
    height: 26px;
    min-width: 64px;
    padding: 0 9px;
    border-radius: 7px;
    font-size: 12.5px;
  }
</style>
