import { read, write } from './storage';

// The upstream repository. Self-hosted copies keep linking to it.
export const REPO = 'dbaio/webhookgo';
export const REPO_URL = `https://github.com/${REPO}`;
export const FORK_URL = `${REPO_URL}/fork`;
export const ISSUES_URL = `${REPO_URL}/issues/new`;
export const SELF_HOST_URL = `${REPO_URL}#hosting`;

export interface RepoStats {
  stars: number;
  forks: number;
}

const KEY = 'wh:repo';
const TTL = 6 * 3600_000; // GitHub allows 60 anonymous API calls per hour and IP
let pending: Promise<RepoStats | null> | undefined;

/** Star and fork counts, fetched once per page and cached for a few hours. */
export function repoStats(): Promise<RepoStats | null> {
  pending ??= (async () => {
    const cached = read<{ at: number; stats: RepoStats | null } | null>(KEY, null);
    if (cached && Date.now() - cached.at < TTL) return cached.stats;
    try {
      const res = await fetch(`https://api.github.com/repos/${REPO}`, { headers: { Accept: 'application/vnd.github+json' } });
      const j = res.ok ? await res.json() : null;
      const stats = j ? { stars: j.stargazers_count, forks: j.forks_count } : null;
      write(KEY, { at: Date.now(), stats });
      return stats;
    } catch {
      return cached?.stats ?? null;
    }
  })();
  return pending;
}

const compact = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
export const count = (n: number) => compact.format(n);
