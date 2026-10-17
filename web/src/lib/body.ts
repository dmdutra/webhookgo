import type { FullRequest } from './types';

export type BodyKind = 'empty' | 'json' | 'xml' | 'form' | 'text' | 'binary' | 'image';

export function detectKind(req: FullRequest): BodyKind {
  if (!req.content) return 'empty';
  const ct = (req.content_type || '').toLowerCase();
  if (req.encoding === 'base64') return ct.startsWith('image/') ? 'image' : 'binary';
  if (ct.includes('x-www-form-urlencoded')) return 'form';
  const head = req.content.trimStart().slice(0, 1);
  if ((ct.includes('json') || head === '{' || head === '[') && parseJson(req.content)) return 'json';
  if ((ct.includes('xml') || head === '<') && !ct.includes('html') && prettyXml(req.content) !== null) return 'xml';
  return 'text';
}

// --- JSON -------------------------------------------------------------------
// A small parser instead of JSON.parse: it keeps number literals verbatim
// (64-bit IDs survive) and object keys in their original order.

export type JNode =
  | { t: 'o'; entries: [string, JNode][] }
  | { t: 'a'; items: JNode[] }
  | { t: 's'; v: string }
  | { t: 'n'; raw: string }
  | { t: 'b'; v: boolean }
  | { t: 'z' };

const cache = new Map<string, JNode | null>();

export function parseJson(text: string): JNode | null {
  if (cache.has(text)) return cache.get(text)!;
  let node: JNode | null = null;
  try {
    node = new Parser(text).document();
  } catch {
    node = null;
  }
  if (cache.size > 50) cache.clear();
  cache.set(text, node);
  return node;
}

class Parser {
  i = 0;
  constructor(private s: string) {}

  document(): JNode {
    const v = this.value(0);
    this.ws();
    if (this.i !== this.s.length) throw new Error('trailing data');
    return v;
  }

  private ws() {
    const s = this.s;
    while (this.i < s.length) {
      const c = s.charCodeAt(this.i);
      if (c === 32 || c === 9 || c === 10 || c === 13) this.i++;
      else break;
    }
  }

  private value(depth: number): JNode {
    if (depth > 400) throw new Error('too deep');
    this.ws();
    const c = this.s[this.i];
    if (c === '{') return this.object(depth);
    if (c === '[') return this.array(depth);
    if (c === '"') return { t: 's', v: this.string() };
    if (this.s.startsWith('true', this.i)) return (this.i += 4), { t: 'b', v: true };
    if (this.s.startsWith('false', this.i)) return (this.i += 5), { t: 'b', v: false };
    if (this.s.startsWith('null', this.i)) return (this.i += 4), { t: 'z' };
    const m = /^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?/.exec(this.s.slice(this.i, this.i + 400));
    if (!m) throw new Error(`unexpected ${c ?? 'end'}`);
    this.i += m[0].length;
    return { t: 'n', raw: m[0] };
  }

  private object(depth: number): JNode {
    this.i++;
    const entries: [string, JNode][] = [];
    this.ws();
    if (this.s[this.i] === '}') return this.i++, { t: 'o', entries };
    for (;;) {
      this.ws();
      if (this.s[this.i] !== '"') throw new Error('expected key');
      const key = this.string();
      this.ws();
      if (this.s[this.i++] !== ':') throw new Error('expected :');
      entries.push([key, this.value(depth + 1)]);
      this.ws();
      const c = this.s[this.i++];
      if (c === '}') return { t: 'o', entries };
      if (c !== ',') throw new Error('expected , or }');
    }
  }

  private array(depth: number): JNode {
    this.i++;
    const items: JNode[] = [];
    this.ws();
    if (this.s[this.i] === ']') return this.i++, { t: 'a', items };
    for (;;) {
      items.push(this.value(depth + 1));
      this.ws();
      const c = this.s[this.i++];
      if (c === ']') return { t: 'a', items };
      if (c !== ',') throw new Error('expected , or ]');
    }
  }

  private string(): string {
    const start = this.i;
    this.i++;
    const s = this.s;
    let simple = true;
    while (this.i < s.length) {
      const c = s.charCodeAt(this.i);
      if (c === 34) break; // "
      if (c === 92) {
        simple = false;
        this.i += 2;
        continue;
      }
      if (c < 32) throw new Error('control character in string');
      this.i++;
    }
    if (this.i >= s.length) throw new Error('unterminated string');
    this.i++;
    const lit = s.slice(start, this.i);
    return simple ? lit.slice(1, -1) : (JSON.parse(lit) as string);
  }
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

export const scalarText = (n: JNode): string =>
  n.t === 's' ? JSON.stringify(n.v) : n.t === 'n' ? n.raw : n.t === 'b' ? String(n.v) : n.t === 'z' ? 'null' : '';

export const scalarClass = (n: JNode) => ({ s: 's', n: 'd', b: 'b', z: 'n', o: '', a: '' })[n.t];

/** Pretty-printed JSON as text. */
export function jsonText(n: JNode, indent = ''): string {
  const inner = indent + '  ';
  if (n.t === 'o') {
    if (!n.entries.length) return '{}';
    return `{\n${n.entries.map(([k, v]) => `${inner}${JSON.stringify(k)}: ${jsonText(v, inner)}`).join(',\n')}\n${indent}}`;
  }
  if (n.t === 'a') {
    if (!n.items.length) return '[]';
    return `[\n${n.items.map((v) => inner + jsonText(v, inner)).join(',\n')}\n${indent}]`;
  }
  return scalarText(n);
}

/** Pretty-printed, syntax-highlighted JSON as escaped HTML. */
export function jsonHtml(n: JNode, indent = ''): string {
  const inner = indent + '  ';
  const p = (s: string) => `<span class="p">${s}</span>`;
  if (n.t === 'o') {
    if (!n.entries.length) return p('{}');
    const rows = n.entries.map(([k, v]) => `${inner}<span class="k">${esc(JSON.stringify(k))}</span>${p(':')} ${jsonHtml(v, inner)}`);
    return `${p('{')}\n${rows.join(`${p(',')}\n`)}\n${indent}${p('}')}`;
  }
  if (n.t === 'a') {
    if (!n.items.length) return p('[]');
    return `${p('[')}\n${n.items.map((v) => inner + jsonHtml(v, inner)).join(`${p(',')}\n`)}\n${indent}${p(']')}`;
  }
  return `<span class="${scalarClass(n)}">${esc(scalarText(n))}</span>`;
}

// --- XML --------------------------------------------------------------------

/** Pretty-printed, highlighted XML as escaped HTML, or null if not well-formed. */
export function prettyXml(text: string): string | null {
  if (typeof DOMParser === 'undefined') return null;
  const doc = new DOMParser().parseFromString(text, 'application/xml');
  if (doc.getElementsByTagName('parsererror').length) return null;

  const lines: string[] = [];
  const decl = /^\s*<\?xml[^?]*\?>/.exec(text);
  if (decl) lines.push(`<span class="xc">${esc(decl[0].trim())}</span>`);

  const tag = (s: string) => `<span class="xt">${s}</span>`;
  const open = (el: Element, selfClose: boolean) => {
    const attrs = Array.from(el.attributes)
      .map((a) => ` <span class="xa">${esc(a.name)}</span>=<span class="xs">"${esc(a.value).replace(/"/g, '&quot;')}"</span>`)
      .join('');
    return `${tag('&lt;' + esc(el.tagName))}${attrs}${tag(selfClose ? '/&gt;' : '&gt;')}`;
  };

  const walk = (node: Node, depth: number) => {
    const pad = '  '.repeat(depth);
    switch (node.nodeType) {
      case Node.ELEMENT_NODE: {
        const el = node as Element;
        const kids = Array.from(el.childNodes).filter((c) => c.nodeType !== Node.TEXT_NODE || c.textContent!.trim() !== '');
        if (!kids.length) {
          lines.push(pad + open(el, true));
        } else if (kids.length === 1 && kids[0].nodeType === Node.TEXT_NODE && !kids[0].textContent!.includes('\n')) {
          lines.push(`${pad}${open(el, false)}${esc(kids[0].textContent!.trim())}${tag('&lt;/' + esc(el.tagName) + '&gt;')}`);
        } else {
          lines.push(pad + open(el, false));
          kids.forEach((k) => walk(k, depth + 1));
          lines.push(pad + tag('&lt;/' + esc(el.tagName) + '&gt;'));
        }
        break;
      }
      case Node.TEXT_NODE:
        lines.push(pad + esc(node.textContent!.trim()));
        break;
      case Node.CDATA_SECTION_NODE:
        lines.push(`${pad}<span class="xc">&lt;![CDATA[</span>${esc(node.textContent!)}<span class="xc">]]&gt;</span>`);
        break;
      case Node.COMMENT_NODE:
        lines.push(`${pad}<span class="xc">&lt;!--${esc(node.textContent!)}--&gt;</span>`);
        break;
      case Node.PROCESSING_INSTRUCTION_NODE: {
        const pi = node as ProcessingInstruction;
        lines.push(`${pad}<span class="xc">&lt;?${esc(pi.target)} ${esc(pi.data)}?&gt;</span>`);
        break;
      }
    }
  };
  doc.childNodes.forEach((n) => walk(n, 0));
  return lines.join('\n');
}

// --- Binary -----------------------------------------------------------------

export function base64ToBytes(b64: string): Uint8Array<ArrayBuffer> {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function bodyBytes(req: FullRequest): Uint8Array<ArrayBuffer> {
  return req.encoding === 'base64' ? base64ToBytes(req.content) : new TextEncoder().encode(req.content);
}

export const HEX_LIMIT = 64 * 1024;

/** Classic offset / hex / ASCII dump of the first HEX_LIMIT bytes. */
export function hexdump(bytes: Uint8Array): string {
  const rows: string[] = [];
  const n = Math.min(bytes.length, HEX_LIMIT);
  for (let off = 0; off < n; off += 16) {
    let hex = '';
    let ascii = '';
    for (let i = 0; i < 16; i++) {
      const b = bytes[off + i];
      if (off + i < n) {
        hex += b.toString(16).padStart(2, '0') + ' ';
        ascii += b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '·';
      } else {
        hex += '   ';
      }
      if (i === 7) hex += ' ';
    }
    rows.push(`${off.toString(16).padStart(8, '0')}  ${hex} ${ascii}`);
  }
  return rows.join('\n');
}
