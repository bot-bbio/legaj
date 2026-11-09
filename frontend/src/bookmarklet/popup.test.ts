import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import popupPage from '../../../internal/clip/static/clip.html?raw';
import popupScript from '../../../internal/clip/static/clip.js?raw';
import { bookmarkletURL } from './bookmarklet';

/**
 * The popup's real page and script (R18, D33), run in jsdom, alone and then
 * together with the real built bookmarklet. Go serves these same two files.
 */

const PORT = 47123;
const POPUP_ORIGIN = `http://127.0.0.1:${PORT}`;
const JOB_ORIGIN = 'https://jobs.example.com';
const SESSION = 'ab'.repeat(16);

const $ = <T extends HTMLElement>(id: string): T => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`no #${id}`);
  return el as T;
};
const status = (): string => $('status').textContent;
const saveButton = (): HTMLButtonElement => $<HTMLButtonElement>('save');

const snapshot = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  type: 'legaj-clip',
  v: 1,
  url: `${JOB_ORIGIN}/jobs/42`,
  title: 'Data Analyst | Contoso',
  jsonld: [],
  meta: {},
  h1: ['Data Analyst'],
  selection: '',
  text: 'The posting',
  ...over,
});

let opener: { postMessage: ReturnType<typeof vi.fn> };
let fetchMock: ReturnType<typeof vi.fn>;
let addListener: ReturnType<typeof vi.spyOn>;

/** Loads the popup's page and runs its script, as a window opened by `from`. */
function openPopup(from: unknown): void {
  const page = new DOMParser().parseFromString(popupPage.replace('{{.Session}}', SESSION), 'text/html');
  document.head.innerHTML = page.head.innerHTML;
  document.body.innerHTML = page.body.innerHTML;
  vi.stubGlobal('opener', from);
  const run = eval;
  run(popupScript);
}

function send(data: unknown, source: unknown = opener, origin = JOB_ORIGIN): void {
  window.dispatchEvent(Object.assign(new Event('message'), { source, origin, data }));
}

function reply(httpStatus: number, body: unknown): void {
  fetchMock.mockResolvedValue(new Response(JSON.stringify(body), { status: httpStatus }));
}

async function save(): Promise<void> {
  saveButton().click();
  await vi.waitFor(() => expect(status()).not.toBe('Saving…'));
}

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'innerText', {
    configurable: true,
    get(this: HTMLElement) {
      return this.textContent;
    },
  });
});

beforeEach(() => {
  opener = { postMessage: vi.fn() };
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  addListener = vi.spyOn(window, 'addEventListener');
});

afterEach(() => {
  // Each run of the script adds a listener; only this test's may hear the next.
  for (const [type, listener] of addListener.mock.calls) {
    if (type === 'message') window.removeEventListener(type, listener as EventListener);
  }
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('R18 popup', () => {
  it('says it is ready, with no data in the message', () => {
    openPopup(opener);
    expect(opener.postMessage).toHaveBeenCalledExactlyOnceWith({ type: 'legaj-ready', v: 1 }, '*');
  });

  it('explains when the job page cut the link (D33)', () => {
    openPopup(null);
    expect(status()).toMatch(/doesn't let LeGaJ read it/);
    expect(status()).toMatch(/Paste job/);
    expect(saveButton().hidden).toBe(true);
  });

  it('says so when the job page never answers', () => {
    vi.useFakeTimers();
    openPopup(opener);
    vi.advanceTimersByTime(8000);
    expect(status()).toMatch(/didn't answer/);
  });

  it('shows the job and waits for a click on Save', () => {
    openPopup(opener);
    send(snapshot());
    expect(status()).toMatch(/^Save this job/);
    expect($('page-title').textContent).toBe('Data Analyst');
    expect($('page-host').textContent).toBe('jobs.example.com');
    expect(saveButton().hidden).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows hostile page text as text', () => {
    openPopup(opener);
    send(snapshot({ h1: ['<img src=x onerror=alert(1)>'] }));
    expect($('page-title').textContent).toBe('<img src=x onerror=alert(1)>');
    expect(document.querySelector('img')).toBeNull();
  });

  it('listens only to the window that opened it', () => {
    openPopup(opener);
    send(snapshot(), { other: true });
    expect(status()).toMatch(/^Waiting/);
    expect(saveButton().hidden).toBe(true);
  });

  it('takes only the first snapshot', () => {
    openPopup(opener);
    send(snapshot());
    send(snapshot({ h1: ['Something else'] }));
    expect($('page-title').textContent).toBe('Data Analyst');
  });

  // The preview and the stored row show this address, and any site can open
  // this window and claim to be LinkedIn. Only the browser's word counts.
  it('refuses a snapshot claiming to come from another site', () => {
    openPopup(opener);
    send(snapshot({ url: 'https://www.linkedin.com/jobs/view/123' }), opener, 'https://evil.example');
    expect(status()).toMatch(/can't read/);
    expect(saveButton().hidden).toBe(true);
  });

  // A sandboxed page's origin is 'null', and so is that of a made-up scheme.
  it('refuses a sandboxed page posing behind a made-up scheme', () => {
    openPopup(opener);
    send(snapshot({ url: 'foo://www.linkedin.com/jobs/view/1' }), opener, 'null');
    expect(status()).toMatch(/can't read/);
    expect(saveButton().hidden).toBe(true);
  });

  it.each([
    ['another version', { v: 2 }],
    ['no address', { url: '' }],
    ['a bad address', { url: 'not a url' }],
    ['a title that is not text', { title: 7 }],
    ['text that is not text', { text: null }],
    ['too many JSON-LD blocks', { jsonld: new Array(21).fill('{}') }],
    ['too many headings', { h1: new Array(6).fill('x') }],
    ['a heading list that is not a list', { h1: 'Data Analyst' }],
    ['meta that is not a map', { meta: ['x'] }],
    ['a meta value that is not text', { meta: { 'og:title': 7 } }],
  ])('refuses %s', (_, over) => {
    openPopup(opener);
    send(snapshot(over));
    expect(status()).toMatch(/can't read/);
    expect(saveButton().hidden).toBe(true);
  });

  it('posts the session and only the fields LeGaJ reads', async () => {
    reply(200, { ok: true, result: { created: true, role: 'Data Analyst', company: 'Contoso' } });
    openPopup(opener);
    send(snapshot({ cookies: 'whatever the page added' }));
    await save();

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/clips');
    expect(init.method).toBe('POST');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
    const body = JSON.parse(init.body as string) as { session: string; page: Record<string, unknown> };
    expect(body.session).toBe(SESSION);
    expect(body.page).toEqual(snapshot());
  });

  it.each([
    ['a new job', { created: true, role: 'Analyst', company: 'Contoso' }, 'Saved to LeGaJ: Analyst at Contoso.'],
    ['a job already saved', { created: false, role: 'Analyst', company: 'Contoso' }, 'Already in your pipeline: Analyst at Contoso.'],
    ['a saved job with no details', { created: false, role: '', company: '' }, 'This job is already in your pipeline.'],
    ['a company being looked up', { created: true, role: 'Analyst', company: '', needsCompany: true, enriching: true }, 'Saved Analyst. LeGaJ is looking up the company name.'],
    ['a company to add by hand', { created: true, role: '', company: '', needsCompany: true, enriching: false }, 'Saved. Add the company name in LeGaJ.'],
  ])('reports %s', async (_, result, text) => {
    reply(200, { ok: true, result });
    openPopup(opener);
    send(snapshot());
    await save();
    expect(status()).toBe(text);
    expect(saveButton().hidden).toBe(true);
  });

  it.each([
    [429, 'Wait a moment and try again.'],
    [500, 'LeGaJ couldn’t save this. Try again.'],
  ])('lets the user try again after a %i', async (code, message) => {
    reply(code, { ok: false, message });
    openPopup(opener);
    send(snapshot());
    await save();
    expect(status()).toBe(message);
    expect(saveButton().disabled).toBe(false);
  });

  it.each([
    [403, 'This clip window has expired. Click the bookmarklet again.'],
    [413, 'This page is too large to clip.'],
    [422, 'LeGaJ found nothing to save on this page.'],
  ])('offers no retry after a %i, which needs a new click on the bookmarklet', async (code, message) => {
    reply(code, { ok: false, message });
    openPopup(opener);
    send(snapshot());
    await save();
    expect(status()).toBe(message);
    expect(saveButton().disabled).toBe(true);
  });

  it('says so when LeGaJ stops answering', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));
    openPopup(opener);
    send(snapshot());
    await save();
    expect(status()).toMatch(/isn't answering/);
  });
});

describe('R18 bookmarklet and popup together', () => {
  it('hands a real snapshot from the job page to the server', async () => {
    document.title = 'Data Analyst | Contoso';
    document.body.innerHTML = '<main><h1>Data Analyst</h1><p>The posting</p></main>';
    history.replaceState(null, '', '/jobs/42');
    const jobURL = location.href;

    // Two windows, joined the way the browser joins them: each message
    // arrives with the sender's window and origin.
    const page = { postMessage: (data: unknown) => send(data, popup, POPUP_ORIGIN) };
    const popup = {
      closed: false,
      close: vi.fn(),
      focus: vi.fn(),
      postMessage: (data: unknown, target: string) => {
        expect(target).toBe(POPUP_ORIGIN);
        send(data, page, location.origin);
      },
    };
    vi.stubGlobal('open', vi.fn(() => popup));
    vi.stubGlobal('alert', vi.fn());

    const script = decodeURIComponent(bookmarkletURL(PORT).slice('javascript:'.length));
    const run = eval;
    run(script);
    openPopup(page);

    expect(status()).toMatch(/^Save this job/);
    expect($('page-title').textContent).toBe('Data Analyst');

    reply(200, { ok: true, result: { created: true, role: 'Data Analyst', company: 'Contoso' } });
    await save();
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const body = JSON.parse(init.body as string) as { session: string; page: Record<string, unknown> };
    expect(body.session).toBe(SESSION);
    expect(body.page).toMatchObject({ type: 'legaj-clip', v: 1, url: jobURL, title: 'Data Analyst | Contoso', h1: ['Data Analyst'] });
    expect(body.page['text']).toContain('The posting');
    expect(status()).toBe('Saved to LeGaJ: Data Analyst at Contoso.');
  });
});
