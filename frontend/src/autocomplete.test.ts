import { parse } from 'svelte/compiler';
import { describe, expect, it } from 'vitest';

// R34: nothing the user types may be kept in the WebView2 folder. WebView2
// saves form values for autofill unless a field or its form opts out, and
// Wails gives the app no way to turn autofill off for the whole WebView. So
// every text field and every form in the app says autocomplete="off".

const sources = import.meta.glob('./**/*.svelte', { query: '?raw', import: 'default', eager: true }) as Record<string, string>;

// Inputs that hold no typed text.
const TEXTLESS = new Set(['checkbox', 'radio', 'file', 'hidden', 'button', 'submit', 'reset', 'range', 'color', 'image']);

interface TextNode {
  type: 'Text';
  data: string;
}
interface Attr {
  type: string;
  name?: string;
  value?: true | TextNode[] | { type: string };
}
interface ElementNode {
  type: string;
  name?: string;
  start: number;
  attributes?: Attr[];
}

/** An attribute's value when it is fixed text; null when it is an expression. */
function staticValue(a: Attr): string | null {
  if (a.value === true) return '';
  if (Array.isArray(a.value) && a.value.every((v) => v.type === 'Text')) return a.value.map((v) => v.data).join('');
  return null;
}

function walk(node: unknown, visit: (n: ElementNode) => void): void {
  if (node === null || typeof node !== 'object') return;
  if (Array.isArray(node)) {
    for (const n of node) walk(n, visit);
    return;
  }
  const n = node as ElementNode & Record<string, unknown>;
  if (typeof n.type === 'string') visit(n);
  for (const [key, value] of Object.entries(n)) {
    if (key !== 'parent' && typeof value === 'object') walk(value, visit);
  }
}

/** Every text field and form, with whether it opts out of autofill. */
function fields(): { where: string; off: boolean }[] {
  const out: { where: string; off: boolean }[] = [];
  for (const [file, src] of Object.entries(sources)) {
    walk(parse(src, { modern: true }).fragment, (n) => {
      if (n.type !== 'RegularElement') return;
      const attrs = (n.attributes ?? []).filter((a) => a.type === 'Attribute');
      const named = (name: string): Attr | undefined => attrs.find((a) => a.name === name);
      if (n.name === 'input') {
        const type = named('type');
        const kind = type ? staticValue(type) : 'text';
        if (kind !== null && TEXTLESS.has(kind)) return;
      } else if (n.name !== 'textarea' && n.name !== 'form') {
        return;
      }
      const ac = named('autocomplete');
      const line = src.slice(0, n.start).split('\n').length;
      out.push({ where: `${file}:${line} <${n.name}>`, off: ac !== undefined && staticValue(ac) === 'off' });
    });
  }
  return out;
}

describe('R34 browser autofill stays off', () => {
  it('finds the app\'s text fields and forms', () => {
    expect(fields().length).toBeGreaterThan(40);
  });

  it('every text field and form opts out, so WebView2 keeps nothing the user types', () => {
    expect(fields().filter((f) => !f.off).map((f) => f.where)).toEqual([]);
  });
});
