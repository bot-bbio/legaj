// A small DevTools-protocol client for the release sweep. It drives LeGaJ's
// WebView2 (started with --remote-debugging-port) and Edge, over Node's
// built-in WebSocket.
import { readFileSync } from 'node:fs';

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** Reads a JSON file. Windows PowerShell writes UTF-8 with a byte-order mark. */
export function readJSON(path) {
  let s = readFileSync(path, 'utf8');
  if (s.charCodeAt(0) === 0xfeff) s = s.slice(1);
  return JSON.parse(s);
}

export async function until(what, fn, ms = 15000, every = 100) {
  const end = Date.now() + ms;
  let last;
  for (;;) {
    try {
      const v = await fn();
      if (v) return v;
    } catch (err) {
      last = err;
    }
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}${last ? `: ${last.message}` : ''}`);
    await sleep(every);
  }
}

export class Devtools {
  constructor(ws) {
    this.ws = ws;
    this.seq = 0;
    this.pending = new Map();
    this.listeners = new Set();
    ws.onmessage = (m) => {
      const msg = JSON.parse(m.data);
      if (msg.id && this.pending.has(msg.id)) {
        const { ok, fail } = this.pending.get(msg.id);
        this.pending.delete(msg.id);
        msg.error ? fail(new Error(JSON.stringify(msg.error))) : ok(msg.result);
      } else {
        for (const fn of this.listeners) fn(msg);
      }
    };
  }

  static async connect(url) {
    const ws = new WebSocket(url);
    await new Promise((ok, fail) => {
      ws.onopen = ok;
      ws.onerror = () => fail(new Error(`could not connect to ${url}`));
    });
    return new Devtools(ws);
  }

  send(method, params = {}, sessionId) {
    const id = ++this.seq;
    this.ws.send(JSON.stringify({ id, method, params, sessionId }));
    return new Promise((ok, fail) => this.pending.set(id, { ok, fail }));
  }

  close() {
    this.ws.close();
  }
}

/** Lists the targets of a DevTools endpoint on a loopback port. */
export async function targets(port) {
  const res = await fetch(`http://127.0.0.1:${port}/json/list`);
  return res.json();
}

/** Connects to the first page target on port whose URL passes match. */
export async function pageOn(port, match = () => true, ms = 60000) {
  const t = await until(`a page target on port ${port}`, async () => (await targets(port)).find((x) => x.type === 'page' && match(x.url)), ms, 500);
  const cdp = await Devtools.connect(t.webSocketDebuggerUrl);
  await cdp.send('Runtime.enable');
  await cdp.send('Page.enable');
  return cdp;
}

/** Evaluates an expression in the page and returns its value. Promises are awaited. */
export async function evaluate(cdp, expression, sessionId) {
  const r = await cdp.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true, userGesture: true }, sessionId);
  if (r.exceptionDetails) {
    throw new Error(`page error: ${r.exceptionDetails.exception?.description ?? r.exceptionDetails.text}`);
  }
  return r.result.value;
}

/** Saves a PNG of the page. */
export async function screenshot(cdp, path, writeFileSync, sessionId) {
  const { data } = await cdp.send('Page.captureScreenshot', { format: 'png' }, sessionId);
  writeFileSync(path, Buffer.from(data, 'base64'));
}
