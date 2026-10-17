import { headerCase } from './format';
import type { FullRequest } from './types';

/** Headers added by proxies or computed by the client that a replay must not copy. */
const SKIP = new Set([
  'host', 'content-length', 'connection', 'accept-encoding', 'transfer-encoding', 'keep-alive', 'te', 'upgrade',
  'x-forwarded-for', 'x-forwarded-proto', 'x-forwarded-host', 'x-forwarded-port', 'x-forwarded-server',
  'x-real-ip', 'via', 'forwarded', 'cdn-loop',
]);

export function replayHeaders(req: FullRequest): [string, string][] {
  return Object.entries(req.headers)
    .filter(([k]) => !SKIP.has(k))
    .sort(([a], [b]) => a.localeCompare(b))
    .flatMap(([k, vs]) => vs.map((v): [string, string] => [headerCase(k), v]));
}

const hasBody = (req: FullRequest) => req.content !== '' && req.method !== 'GET' && req.method !== 'HEAD';
const sh = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;
// HTTP methods may contain ` $ | & ' — quote anything that isn't a plain word.
const shMethod = (m: string) => (/^[A-Z][A-Z0-9_]*$/.test(m) ? m : sh(m));
const str = (s: string) => JSON.stringify(s); // valid in JS, Python and Go

export const SNIPPETS = {
  curl: 'cURL',
  httpie: 'HTTPie',
  fetch: 'JavaScript fetch',
  python: 'Python requests',
  go: 'Go net/http',
  http: 'Raw HTTP',
} as const;
export type SnippetKind = keyof typeof SNIPPETS;

export function snippet(kind: SnippetKind, req: FullRequest, url = req.url): string {
  const headers = replayHeaders(req);
  const body = hasBody(req);
  const binary = req.encoding === 'base64';

  switch (kind) {
    case 'curl': {
      const parts = [req.method === 'GET' && !body ? `curl ${sh(url)}` : `curl -X ${shMethod(req.method)} ${sh(url)}`];
      for (const [k, v] of headers) parts.push(`  -H ${sh(`${k}: ${v}`)}`);
      if (body && binary) parts.push(`  --data-binary @-`);
      else if (body) parts.push(`  --data-raw ${sh(req.content)}`);
      const cmd = parts.join(' \\\n');
      return body && binary ? `echo ${sh(req.content)} | base64 -d | ${cmd}` : cmd;
    }
    case 'httpie': {
      const parts = [`http ${shMethod(req.method)} ${sh(url)}`];
      for (const [k, v] of headers) parts.push(`  ${sh(`${k}:${v}`)}`);
      if (body && !binary) parts.push(`  --raw ${sh(req.content)}`);
      const cmd = parts.join(' \\\n');
      return body && binary ? `echo ${sh(req.content)} | base64 -d | ${cmd}` : cmd;
    }
    case 'fetch': {
      const lines = [`const response = await fetch(${str(url)}, {`, `  method: ${str(req.method)},`];
      if (headers.length) {
        lines.push('  headers: {');
        // fetch takes one value per name: repeated headers are comma-joined.
        const merged = new Map<string, string[]>();
        for (const [k, v] of headers) merged.set(k, [...(merged.get(k) ?? []), v]);
        for (const [k, vs] of merged) lines.push(`    ${str(k)}: ${str(vs.join(', '))},`);
        lines.push('  },');
      }
      if (body) lines.push(binary ? `  body: Uint8Array.from(atob(${str(req.content)}), (c) => c.charCodeAt(0)),` : `  body: ${str(req.content)},`);
      lines.push('});', 'console.log(response.status, await response.text());');
      return lines.join('\n');
    }
    case 'python': {
      const lines = binary && body ? ['import base64', 'import requests', ''] : ['import requests', ''];
      lines.push('response = requests.request(', `    ${str(req.method)},`, `    ${str(url)},`);
      if (headers.length) {
        lines.push('    headers={');
        const merged = new Map<string, string[]>();
        for (const [k, v] of headers) merged.set(k, [...(merged.get(k) ?? []), v]);
        for (const [k, vs] of merged) lines.push(`        ${str(k)}: ${str(vs.join(', '))},`);
        lines.push('    },');
      }
      if (body) lines.push(binary ? `    data=base64.b64decode(${str(req.content)}),` : `    data=${str(req.content)}.encode(),`);
      lines.push(')', 'print(response.status_code, response.text)');
      return lines.join('\n');
    }
    case 'go': {
      const imports = ['"fmt"', '"io"', '"net/http"'];
      let bodyExpr = 'nil';
      const pre: string[] = [];
      if (body && binary) {
        imports.unshift('"bytes"', '"encoding/base64"');
        pre.push(`\tdata, _ := base64.StdEncoding.DecodeString(${str(req.content)})`);
        bodyExpr = 'bytes.NewReader(data)';
      } else if (body) {
        imports.push('"strings"');
        bodyExpr = `strings.NewReader(${str(req.content)})`;
      }
      const lines = ['package main', '', 'import (', ...imports.sort().map((i) => `\t${i}`), ')', '', 'func main() {', ...pre];
      lines.push(`\treq, err := http.NewRequest(${str(req.method)}, ${str(url)}, ${bodyExpr})`, '\tif err != nil {', '\t\tpanic(err)', '\t}');
      for (const [k, v] of headers) lines.push(`\treq.Header.Add(${str(k)}, ${str(v)})`);
      lines.push(
        '\tresp, err := http.DefaultClient.Do(req)', '\tif err != nil {', '\t\tpanic(err)', '\t}', '\tdefer resp.Body.Close()',
        '\tb, _ := io.ReadAll(resp.Body)', '\tfmt.Println(resp.Status, string(b))', '}',
      );
      return lines.join('\n');
    }
    case 'http': {
      const u = new URL(url);
      const lines = [`${req.method} ${u.pathname}${u.search} ${req.protocol || 'HTTP/1.1'}`];
      for (const [k, vs] of Object.entries(req.headers).sort(([a], [b]) => a.localeCompare(b)))
        for (const v of vs) lines.push(`${headerCase(k)}: ${v}`);
      lines.push('');
      if (req.content) lines.push(binary ? `[binary body, base64]\n${req.content}` : req.content);
      return lines.join('\r\n');
    }
  }
}

// --- "Send your first request" examples ---------------------------------------

export const SAMPLES = {
  curl: 'cURL',
  httpie: 'HTTPie',
  fetch: 'JavaScript',
  python: 'Python',
  powershell: 'PowerShell',
} as const;
export type SampleKind = keyof typeof SAMPLES;

export function sample(kind: SampleKind, url: string): string {
  switch (kind) {
    case 'curl':
      return `curl -X POST ${url} \\\n  -H 'Content-Type: application/json' \\\n  -d '{"event": "order.created", "id": 1234, "amount": 49.9}'`;
    case 'httpie':
      return `http POST ${url} event=order.created id:=1234 amount:=49.9`;
    case 'fetch':
      return `await fetch("${url}", {\n  method: "POST",\n  headers: { "Content-Type": "application/json" },\n  body: JSON.stringify({ event: "order.created", id: 1234, amount: 49.9 }),\n});`;
    case 'python':
      return `import requests\n\nrequests.post("${url}", json={"event": "order.created", "id": 1234, "amount": 49.9})`;
    case 'powershell':
      return `Invoke-RestMethod -Method Post -Uri "${url}" \`\n  -ContentType "application/json" \`\n  -Body '{"event": "order.created", "id": 1234, "amount": 49.9}'`;
  }
}
