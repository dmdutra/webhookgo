<script lang="ts">
  import Bug from '@lucide/svelte/icons/bug';
  import GitFork from '@lucide/svelte/icons/git-fork';
  import ServerCog from '@lucide/svelte/icons/server-cog';
  import Star from '@lucide/svelte/icons/star';
  import { count, FORK_URL, ISSUES_URL, REPO_URL, repoStats, SELF_HOST_URL, type RepoStats } from '../lib/project';
  import GitHubMark from './GitHubMark.svelte';

  let stats = $state<RepoStats | null>(null);
  repoStats().then((s) => (stats = s));
</script>

<section class="oss">
  <p>
    <GitHubMark size={18} />
    <span><strong>webhookgo</strong> is free and open source. Like it? Star it on GitHub, or run your own with one command.</span>
  </p>
  <div class="actions">
    <a class="btn sm" href={REPO_URL} target="_blank" rel="noopener">
      <Star /> Star{#if stats}<span class="count">{count(stats.stars)}</span>{/if}
    </a>
    <a class="btn sm" href={FORK_URL} target="_blank" rel="noopener">
      <GitFork /> Fork{#if stats}<span class="count">{count(stats.forks)}</span>{/if}
    </a>
    <a class="btn ghost sm" href={SELF_HOST_URL} target="_blank" rel="noopener"><ServerCog /> Self-host</a>
    <a class="btn ghost sm" href={ISSUES_URL} target="_blank" rel="noopener"><Bug /> Report an issue</a>
  </div>
</section>

<style>
  .oss {
    display: grid;
    justify-items: center;
    gap: 12px;
    width: 100%;
    margin-top: 10px;
    padding-top: 22px;
    border-top: 1px solid var(--border);
  }
  p {
    display: flex;
    align-items: center;
    gap: 8px;
    max-width: 520px;
    color: var(--text-2);
    font-size: 13px;
    text-align: left;
  }
  p :global(svg) {
    flex: none;
    color: var(--text);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 8px;
  }
  .count {
    margin-left: 2px;
    padding: 2px 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--text-2);
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }
</style>
