import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import { bookmarkletURL } from './bookmarklet';
import type { ClipPayload } from './collector';

const PORT = 47123;
const ORIGIN = `http://127.0.0.1:${PORT}`;
/** clip.MaxPayload in internal/clip/server.go. */
const MAX_PAYLOAD = 2 << 20;

interface FakePopup {
  closed: boolean;
  close: ReturnType<typeof vi.fn>;
  focus: ReturnType<typeof vi.fn>;
  postMessage: ReturnType<typeof vi.fn>;
}

let popup: FakePopup;
let open: ReturnType<typeof vi.fn>;
let alert: ReturnType<typeof vi.fn>;

/** What a browser does with the bookmark: decode the URL, then run the script. */
function clickBookmark(port = PORT): unknown {
  const url = bookmarkletURL(port);
  const script = decodeURIComponent(url.slice('javascript:'.length));
  const run = eval; // indirect, so it runs in the global scope like a page script
  return run(script);
}

function arrive(source: unknown, origin: string, data: unknown): void {
  window.dispatchEvent(Object.assign(new Event('message'), { source, origin, data }));
}

/** Clicks the bookmark, lets the popup say ready, and returns what it was sent. */
function capture(): ClipPayload {
  clickBookmark();
  arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
  expect(popup.postMessage).toHaveBeenCalledTimes(1);
  return popup.postMessage.mock.calls[0]?.[0] as ClipPayload;
}

/** The bytes the popup posts: the session and the page, as JSON in UTF-8. */
function wireBytes(page: ClipPayload): number {
  return new TextEncoder().encode(JSON.stringify({ session: '0'.repeat(32), page })).length;
}

beforeAll(() => {
  // jsdom has no layout, so no innerText; the tests read textContent instead.
  Object.defineProperty(HTMLElement.prototype, 'innerText', {
    configurable: true,
    get(this: HTMLElement) {
      return this.textContent;
    },
  });
});

beforeEach(() => {
  document.head.innerHTML = '';
  document.body.innerHTML = '';
  document.title = '';
  history.replaceState(null, '', '/jobs/42?ref=mail');
  popup = { closed: false, close: vi.fn(), focus: vi.fn(), postMessage: vi.fn() };
  open = vi.fn(() => popup);
  alert = vi.fn();
  vi.stubGlobal('open', open);
  vi.stubGlobal('alert', alert);
});

afterEach(() => {
  window.getSelection()?.removeAllRanges();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('R18 bookmarklet URL', () => {
  it('is a javascript: URL with no token in it (A16)', () => {
    const url = bookmarkletURL(PORT);
    expect(url.startsWith('javascript:')).toBe(true);
    const script = decodeURIComponent(url.slice('javascript:'.length));
    expect(script).not.toMatch(/session|token|nonce|secret|api.?key/i);
    expect(script).not.toMatch(/[0-9a-f]{32}/i);
  });

  it('builds in the persisted port, everywhere the origin is used', () => {
    const script = decodeURIComponent(bookmarkletURL(47999).slice('javascript:'.length));
    expect(script).toContain('http://127.0.0.1:47999');
    expect(script).not.toContain('__LEGAJ_ORIGIN__');
  });

  it('survives being a URL: nothing in it can end or change it early', () => {
    expect(bookmarkletURL(PORT).slice('javascript:'.length)).not.toMatch(/[\s#"<>]/);
  });

  it('stays small enough to sit in a bookmark', () => {
    expect(bookmarkletURL(PORT).length).toBeLessThan(4096);
  });

  it.each([0, -1, 65536, 1.5, Number.NaN])('refuses the port %s', (port) => {
    expect(() => bookmarkletURL(port)).toThrow();
  });
});

describe('R18 handoff', () => {
  it('opens the popup on the persisted port', () => {
    clickBookmark(47555);
    expect(open).toHaveBeenCalledExactlyOnceWith('http://127.0.0.1:47555/clip', 'legaj_clip', 'popup,width=560,height=680');
    expect(popup.focus).toHaveBeenCalled();
  });

  it('leaves the job page alone: a javascript: URL that returns a value replaces the page', () => {
    expect(clickBookmark()).toBeUndefined();
  });

  it('sends the snapshot to the popup once it is ready, addressed to its origin only', () => {
    clickBookmark();
    expect(popup.postMessage).not.toHaveBeenCalled();
    arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
    expect(popup.postMessage).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'legaj-clip', v: 1 }), ORIGIN);
  });

  it('sends it only once, even if the popup says ready twice', () => {
    clickBookmark();
    arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
    arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
    expect(popup.postMessage).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['another window', { other: true }, ORIGIN, { type: 'legaj-ready', v: 1 }],
    ['another origin', null, 'https://evil.example', { type: 'legaj-ready', v: 1 }],
    ['another message', null, ORIGIN, { type: 'hello' }],
    ['a message with no data', null, ORIGIN, null],
  ])('ignores %s', (_, source, origin, data) => {
    clickBookmark();
    arrive(source ?? popup, origin, data);
    expect(popup.postMessage).not.toHaveBeenCalled();
  });

  it('says so when the browser blocks the popup', () => {
    open.mockReturnValue(null);
    clickBookmark();
    expect(alert).toHaveBeenCalledOnce();
    expect(alert.mock.calls[0]?.[0]).toMatch(/blocked/i);
  });

  it('closes the popup and says so when LeGaJ never answers', () => {
    vi.useFakeTimers();
    clickBookmark();
    vi.advanceTimersByTime(7999);
    expect(alert).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(popup.close).toHaveBeenCalledOnce();
    expect(alert.mock.calls[0]?.[0]).toMatch(/running/i);
  });

  it('stops listening once it has given up', () => {
    vi.useFakeTimers();
    clickBookmark();
    vi.advanceTimersByTime(8000);
    arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
    expect(popup.postMessage).not.toHaveBeenCalled();
  });

  it('stays quiet once the popup answered', () => {
    vi.useFakeTimers();
    clickBookmark();
    arrive(popup, ORIGIN, { type: 'legaj-ready', v: 1 });
    vi.advanceTimersByTime(60000);
    expect(alert).not.toHaveBeenCalled();
    expect(popup.close).not.toHaveBeenCalled();
  });

  // A page sending COOP cuts the link, which shows here as a closed window.
  // The popup is really open and already explains what to do (D33).
  it('leaves a popup it was cut off from to explain itself', () => {
    vi.useFakeTimers();
    clickBookmark();
    popup.closed = true;
    vi.advanceTimersByTime(8000);
    expect(alert).not.toHaveBeenCalled();
    expect(popup.close).not.toHaveBeenCalled();
  });
});

describe('R19 collector', () => {
  it('sends exactly the fields of protocol v1', () => {
    expect(Object.keys(capture()).sort()).toEqual(['h1', 'jsonld', 'meta', 'selection', 'text', 'title', 'type', 'url', 'v']);
  });

  it('gathers the title, the URL and the headings', () => {
    document.title = 'Analyst at Babbage & Co';
    document.body.innerHTML = '<h1> Analyst </h1><div><h1>Babbage &amp; Co</h1></div>';
    const page = capture();
    expect(page.title).toBe('Analyst at Babbage & Co');
    expect(page.url).toBe(location.href);
    expect(page.url).toContain('/jobs/42?ref=mail');
    expect(page.h1).toEqual(['Analyst', 'Babbage & Co']);
  });

  it('gathers JSON-LD blocks and no other scripts', () => {
    document.head.innerHTML = `
      <script type="application/ld+json">{"@type":"JobPosting","title":"Analyst"}</script>
      <script type="application/json">{"private":true}</script>
      <script>var x = 1;</script>
      <script type="application/ld+json">{"@type":"Organization"}</script>`;
    expect(capture().jsonld).toEqual(['{"@type":"JobPosting","title":"Analyst"}', '{"@type":"Organization"}']);
  });

  it('keeps only the meta tags the extractor reads', () => {
    document.head.innerHTML = `
      <meta property="og:title" content="Analyst">
      <meta name="twitter:title" content="Analyst, on Twitter">
      <meta name="description" content="A job">
      <meta name="author" content="Babbage">
      <meta name="viewport" content="width=device-width">
      <meta name="authorization" content="abc123">
      <meta name="csrf-token" content="abc123">`;
    expect(capture().meta).toEqual({
      'og:title': 'Analyst',
      'twitter:title': 'Analyst, on Twitter',
      description: 'A job',
      author: 'Babbage',
    });
  });

  it('gathers the selection', () => {
    document.body.innerHTML = '<p>Before</p><p id="jd">The job description</p>';
    const jd = document.getElementById('jd');
    if (!jd) throw new Error('fixture');
    window.getSelection()?.selectAllChildren(jd);
    expect(capture().selection).toBe('The job description');
  });

  it('reads the main element when there is one, so headers and menus stay out', () => {
    document.body.innerHTML = '<header>Signed in as Ada</header><main>The posting</main>';
    expect(capture().text).toBe('The posting');
  });

  it('falls back to the whole body', () => {
    document.body.innerHTML = '<div>The posting</div>';
    expect(capture().text).toBe('The posting');
  });
});

describe('R18 size budget', () => {
  it('stays within what the server takes on a page full of everything', () => {
    const wide = '€'; // three bytes in UTF-8
    document.head.innerHTML =
      Array.from({ length: 20 }, () => `<script type="application/ld+json">"${wide.repeat(15000)}"</script>`).join('') +
      Array.from({ length: 100 }, (_, i) => `<meta name="og:${i}" content="${wide.repeat(2000)}">`).join('');
    document.body.innerHTML = `<main>${wide.repeat(1_000_000)}</main>`;
    const page = capture();
    expect(wireBytes(page)).toBeLessThanOrEqual(MAX_PAYLOAD);
    expect(page.text.length).toBeGreaterThan(1000);
    expect(page.jsonld.length).toBeGreaterThan(0);
  });

  // JSON writes a control character as six bytes, the worst case for text.
  it('counts the bytes JSON will really use', () => {
    document.body.innerHTML = `<main>${String.fromCharCode(1).repeat(300000)}</main>`;
    document.head.innerHTML = `<script type="application/ld+json">"${'x'.repeat(900000)}"</script>`;
    const page = capture();
    expect(wireBytes(page)).toBeLessThanOrEqual(MAX_PAYLOAD);
    expect(page.text.length).toBeGreaterThan(0);
  });

  // A cut block can't be parsed, and "select the description" can't help
  // when JSON-LD alone is too big, so an oversized block goes whole.
  it('drops an oversized JSON-LD block whole and keeps the ones after it', () => {
    document.head.innerHTML = `
      <script type="application/ld+json">{"junk":"${'x'.repeat(1_500_000)}"}</script>
      <script type="application/ld+json">{"@type":"JobPosting"}</script>`;
    document.body.innerHTML = '<main>The posting</main>';
    const page = capture();
    expect(page.jsonld).toEqual(['{"@type":"JobPosting"}']);
    expect(page.text).toBe('The posting');
  });

  it('caps the lists at what the server accepts', () => {
    document.head.innerHTML =
      Array.from({ length: 30 }, (_, i) => `<script type="application/ld+json">{"n":${i}}</script>`).join('') +
      Array.from({ length: 150 }, (_, i) => `<meta name="og:${i}" content="v">`).join('');
    document.body.innerHTML = Array.from({ length: 9 }, (_, i) => `<h1>Heading ${i}</h1>`).join('');
    const page = capture();
    expect(page.jsonld).toHaveLength(20);
    expect(page.jsonld[0]).toBe('{"n":0}');
    expect(Object.keys(page.meta)).toHaveLength(100);
    expect(page.h1).toEqual(['Heading 0', 'Heading 1', 'Heading 2', 'Heading 3', 'Heading 4']);
  });

  it('caps the long fields', () => {
    document.head.innerHTML = `<meta name="description" content="${'d'.repeat(3000)}">`;
    document.title = 't'.repeat(5000);
    document.body.innerHTML = `<main id="m">${'m'.repeat(400000)}</main><h1>${'h'.repeat(3000)}</h1>`;
    const main = document.getElementById('m');
    if (!main) throw new Error('fixture');
    window.getSelection()?.selectAllChildren(main);
    const page = capture();
    expect(page.title).toHaveLength(2000);
    expect(page.meta['description']).toHaveLength(2000);
    expect(page.h1[0]).toHaveLength(1000);
    expect(page.selection).toHaveLength(20000);
    expect(page.text).toHaveLength(300000);
  });

  // The server refuses a URL over 8192 characters outright, which would lose
  // the whole clip over a tracking query string.
  it('drops the query from a URL the server would refuse', () => {
    history.replaceState(null, '', `/jobs/42?q=${'x'.repeat(9000)}`);
    expect(capture().url).toBe(`${location.origin}/jobs/42`);
  });
});
