// Forwarding replays a captured request from the browser, so it can reach
// targets the server cannot, such as http://localhost on your machine.
import { base64ToBytes } from './body';
import type { FullRequest } from './types';

export interface ForwardSettings {
  url: string;
  method: string; // '' keeps the original method
  appendPath: boolean; // add the path after the token and the query string
  headers: 'all' | 'none' | 'list';
  headerList: string; // comma separated, for headers = 'list'
  auto: boolean; // forward every new request as it arrives
}

export const defaultForward: ForwardSettings = {
  url: '',
  method: '',
  appendPath: true,
  headers: 'all',
  headerList: '',
  auto: false,
};

export interface ForwardResult {
  ok: boolean;
  status?: number;
  statusText?: string;
  error?: string;
  ms: number;
  url: string;
  at: number;
}

// Headers browsers refuse to set (Fetch spec "forbidden request headers"),
// plus proxy artefacts that make no sense to replay.
const FORBIDDEN = new Set([
  'accept-charset', 'accept-encoding', 'access-control-request-headers', 'access-control-request-method',
  'connection', 'content-length', 'cookie', 'cookie2', 'date', 'dnt', 'expect', 'host', 'keep-alive', 'origin',
  'referer', 'set-cookie', 'te', 'trailer', 'transfer-encoding', 'upgrade', 'via', 'user-agent',
  'x-forwarded-for', 'x-forwarded-proto', 'x-forwarded-host', 'x-forwarded-port', 'x-real-ip', 'forwarded',
]);
const forbidden = (name: string) => FORBIDDEN.has(name) || name.startsWith('sec-') || name.startsWith('proxy-');

export function forwardUrl(s: ForwardSettings, req: FullRequest): string {
  const base = s.url.trim();
  if (!s.appendPath) return base;
  let search = '';
  try {
    search = new URL(req.url).search;
  } catch {
    /* keep empty */
  }
  const path = req.path === '/' ? '' : req.path;
  return (path ? base.replace(/\/+$/, '') + path : base) + search;
}

export async function forward(req: FullRequest, s: ForwardSettings): Promise<ForwardResult> {
  const url = forwardUrl(s, req);
  const started = performance.now();
  const done = (r: Omit<ForwardResult, 'ms' | 'url' | 'at'>): ForwardResult => ({
    ...r,
    ms: Math.round(performance.now() - started),
    url,
    at: Date.now(),
  });

  try {
    new URL(url);
  } catch {
    return done({ ok: false, error: `Invalid target URL: ${url || '(empty)'}` });
  }

  const headers = new Headers();
  if (s.headers !== 'none') {
    const allow =
      s.headers === 'list'
        ? new Set(s.headerList.split(',').map((h) => h.trim().toLowerCase()).filter(Boolean))
        : null;
    for (const [name, values] of Object.entries(req.headers)) {
      if (forbidden(name) || (allow && !allow.has(name))) continue;
      for (const v of values) {
        try {
          headers.append(name, v);
        } catch {
          /* invalid for fetch */
        }
      }
    }
  }

  const method = (s.method || req.method).toUpperCase();
  let body: BodyInit | undefined;
  if (method !== 'GET' && method !== 'HEAD' && req.content) {
    body = req.encoding === 'base64' ? base64ToBytes(req.content) : req.content;
  }

  try {
    const res = await fetch(url, { method, headers, body, mode: 'cors', credentials: 'omit', cache: 'no-store' });
    return done({ ok: res.ok, status: res.status, statusText: res.statusText });
  } catch {
    return done({
      ok: false,
      error: 'Network error or blocked by CORS — the target must be reachable from this browser and allow cross-origin requests.',
    });
  }
}
