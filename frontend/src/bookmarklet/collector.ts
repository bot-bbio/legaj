/**
 * The bookmarklet (R18, R19): it runs on the job page, takes a snapshot of
 * it and hands the snapshot to LeGaJ's popup with postMessage (D15, D33).
 *
 * This file is built into the user's bookmark (vite-plugin-bookmarklet.ts),
 * where it stays until they reinstall it, and it runs inside other people's
 * pages. So it holds nothing secret, imports nothing from the app, and only
 * reads the page. Everything LeGaJ decides about a job happens in the app.
 *
 * The limits below follow protocol v1 as the server enforces it
 * (internal/clip/server.go): going over one loses the whole clip.
 */

/** The page snapshot, protocol v1. */
export interface ClipPayload {
  type: 'legaj-clip';
  v: 1;
  url: string;
  title: string;
  jsonld: string[];
  meta: Record<string, string>;
  h1: string[];
  selection: string;
  text: string;
}

/** What the server takes is 2 MiB; the rest is room for the session around the page. */
const MAX_BYTES = 1_900_000;
/** JSON-LD gets about half, so a page stuffed with it still keeps its text. */
const MAX_JSONLD_BYTES = 1_000_000;

const MAX_JSONLD = 20;
const MAX_META = 100;
const MAX_HEADINGS = 5;
const MAX_URL = 8192;
const MAX_TEXT = 300000;
const MAX_SELECTION = 20000;

/** Bytes on the wire: JSON's escapes included, as UTF-8. */
const bytes = (value: unknown): number => new TextEncoder().encode(JSON.stringify(value)).length;

function collectMeta(d: Document): Record<string, string> {
  const meta: Record<string, string> = {};
  d.querySelectorAll('meta[property],meta[name]').forEach((m) => {
    const key = m.getAttribute('property') || m.getAttribute('name') || '';
    if (/^(og:|twitter:|description$|title$|author$)/i.test(key) && Object.keys(meta).length < MAX_META) {
      meta[key] = (m.getAttribute('content') || '').slice(0, 2000);
    }
  });
  return meta;
}

/** Reads the page into a snapshot that fits what the server accepts. */
function collect(): ClipPayload {
  const d = document;
  const href = location.href;
  const page: ClipPayload = {
    type: 'legaj-clip',
    v: 1,
    url: href.length > MAX_URL ? href.replace(/[?#].*$/, '') : href,
    title: d.title.slice(0, 2000),
    jsonld: [],
    meta: collectMeta(d),
    h1: Array.from(d.querySelectorAll('h1'))
      .slice(0, MAX_HEADINGS)
      .map((h) => (h.innerText || '').trim().slice(0, 1000)),
    selection: String(window.getSelection() || '').slice(0, MAX_SELECTION),
    text: '',
  };
  // A block that doesn't fit is left out whole: a cut one can't be parsed.
  d.querySelectorAll('script[type="application/ld+json"]').forEach((s) => {
    const block = s.textContent || '';
    if (page.jsonld.length < MAX_JSONLD && bytes(page) + bytes(block) <= MAX_JSONLD_BYTES) {
      page.jsonld.push(block);
    }
  });
  const root = d.querySelector<HTMLElement>('main') || d.body;
  page.text = root ? (root.innerText || '').slice(0, MAX_TEXT) : '';
  while (page.text && bytes(page) > MAX_BYTES) {
    page.text = page.text.slice(0, Math.floor(page.text.length * 0.75));
  }
  return page;
}

/** Opens the popup and sends it the snapshot when it says it's ready. */
export function clip(origin: string): void {
  const page = collect();
  const popup = window.open(origin + '/clip', 'legaj_clip', 'popup,width=560,height=680');
  if (!popup) {
    window.alert('LeGaJ: your browser blocked the clip window. Allow pop-ups for this site, then click the bookmarklet again.');
    return;
  }
  popup.focus();
  const giveUp = setTimeout(() => {
    window.removeEventListener('message', onMessage);
    // A closed window here isn't always closed: a page that sends COOP cuts
    // the link, and the popup behind it explains what to do (D33).
    if (!popup.closed) {
      popup.close();
      window.alert("LeGaJ didn't answer. Make sure it's running, then click the bookmarklet again.");
    }
  }, 8000);
  const onMessage = (e: MessageEvent): void => {
    if (e.source !== popup || e.origin !== origin || !e.data || e.data.type !== 'legaj-ready') return;
    clearTimeout(giveUp);
    window.removeEventListener('message', onMessage);
    popup.postMessage(page, origin);
  };
  window.addEventListener('message', onMessage);
}
