// The release sweep driver (docs/RELEASE_CHECKLIST.md), for a disposable
// Windows runner only. sweep.ps1 prepares the machine and starts it.
//
// It installs LeGaJ and drives the real app the way a user does: through
// its window, with UI Automation (buttons, fields, the file dialog) and the
// real mouse and keyboard. The shipped app can't be driven any other way:
// Wails' WebView2 loader overwrites WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS,
// so no outside process can turn on remote debugging. Results are read back
// from the app's database (read-only), the file system and Windows.
//
// Edge is driven over the DevTools protocol, with a throwaway profile, and
// its favorites bar with UI Automation and the real mouse. Every check is
// recorded as PASS, FAIL, SKIP or INFO with a reason; one failure doesn't
// stop the checks after it.
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { createServer as createHttpServer } from 'node:http';
import { createServer as createTcpServer } from 'node:net';
import { basename, join } from 'node:path';
import { createInterface } from 'node:readline';
import { Devtools, evaluate, readJSON, sleep, until } from './cdp.mjs';

const cfg = readJSON(process.argv[2]);
const EDGE = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe';
const DATA = join(process.env.LOCALAPPDATA, 'LeGaJ');
const DB = join(DATA, 'legaj.db');
const WEBVIEW_DATA = join(process.env.APPDATA, 'legaj.exe');
const RUN_KEY = 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run';
const UNINSTALLER = join(process.env.LOCALAPPDATA, 'Programs', 'LeGaJ', 'unins000.exe');
const START_MENU = join(process.env.APPDATA, 'Microsoft', 'Windows', 'Start Menu', 'Programs', 'LeGaJ.lnk');
const EDGE_TOP = 500; // Edge sits under LeGaJ's window, its favorites bar in view
const shots = join(cfg.out, 'shots');
mkdirSync(shots, { recursive: true });

// ---------------------------------------------------------------- results

const results = [];
function record(id, item, status, detail = '') {
  results.push({ id, item, status, detail: String(detail).replace(/\s+/g, ' ').slice(0, 700) });
  console.log(`${status.padEnd(4)} ${id} | ${item} | ${detail}`);
}
async function check(id, item, fn) {
  try {
    const r = await fn();
    if (r === undefined || r === true) record(id, item, 'PASS');
    else if (r === false) record(id, item, 'FAIL');
    else record(id, item, r.status, r.detail);
  } catch (err) {
    const wins = await win.call('windows', { name: 'legaj' }).catch(() => []);
    const running = (await win.call('pids', { name: 'legaj' }).catch(() => [])).length > 0;
    const note = !running ? ' [LeGaJ is not running]' : wins.some((w) => w.Hung) ? ' [Windows reports the LeGaJ window as not responding]' : '';
    record(id, item, 'FAIL', `${err.message}${note}`);
    await shot(`fail-${id}`).catch(() => {});
  }
}
const pass = (detail) => ({ status: 'PASS', detail });
const fail = (detail) => ({ status: 'FAIL', detail });
const skip = (detail) => ({ status: 'SKIP', detail });

// ------------------------------------------------------- Windows helper

class Shell {
  constructor() {
    this.ps = spawn('powershell.exe', ['-NoProfile', '-NoLogo', '-ExecutionPolicy', 'Bypass', '-Command', '-'], { stdio: ['pipe', 'pipe', 'pipe'] });
    this.waiting = [];
    createInterface({ input: this.ps.stdout }).on('line', (line) => {
      if (!line.startsWith('{"ok":')) return;
      const next = this.waiting.shift();
      let reply;
      try {
        reply = JSON.parse(line);
      } catch {
        reply = { ok: false, error: `unreadable reply: ${line.slice(0, 200)}` };
      }
      if (next) next(reply);
    });
    this.ps.stderr.on('data', (d) => process.stderr.write(`[ps] ${d}`));
    this.chain = Promise.resolve();
    this.ps.stdin.write(`. '${join(cfg.repo, 'scripts', 'sweep', 'win.ps1')}'\n`);
  }
  call(op, args = {}) {
    const run = () => new Promise((resolve, reject) => {
      const json = JSON.stringify({ op, timeout: 15000, type: '', ...args }).replaceAll("'", "''");
      this.waiting.push((r) => (r.ok ? resolve(r.value) : reject(new Error(`${op}: ${r.error}`))));
      this.ps.stdin.write(`Invoke-Sweep '${json}'\n`);
    });
    const p = this.chain.then(run, run);
    this.chain = p.catch(() => {});
    return p;
  }
  close() {
    this.ps.stdin.end();
  }
}

const win = new Shell();
let shotCount = 0;
const shot = (name) => win.call('screen', { path: join(shots, `${String(++shotCount).padStart(2, '0')}-${name}.png`) });

// --------------------------------------------------- the app, as a user

const APP = 'LeGaJ';

/** A control in LeGaJ's window, found by its accessible name; acts on it as asked. */
function ui(name, action = 'find', opts = {}) {
  return win.call('ui', { window: APP, name, action, enabled: action === 'invoke' || action === 'focus', ...opts });
}
const press = (name, opts = {}) => ui(name, 'invoke', { type: 'Button', ...opts });
const appText = () => win.call('ui-text', { window: APP, timeout: 5000 });
async function waitText(re, ms = 20000) {
  return until(`LeGaJ to show ${re}`, async () => re.test(await appText()), ms, 400);
}
async function waitNoText(re, ms = 20000) {
  return until(`LeGaJ to stop showing ${re}`, async () => !re.test(await appText()), ms, 400);
}

async function legajWindow(ms = 20000) {
  return until('the LeGaJ window', async () => (await win.call('windows', { name: 'legaj' })).find((w) => w.Title === APP), ms, 300);
}
const legajPids = () => win.call('pids', { name: 'legaj' });

async function focusApp() {
  const w = await legajWindow();
  if (!(await win.call('focus', { hwnd: w.Hwnd }))) {
    throw new Error(`LeGaJ could not be brought to the front (foreground: ${(await win.call('foreground')).Title})`);
  }
  await sleep(200);
}

/** Types into a field the way a user pastes: focus it, select all, paste. */
async function fill(name, text, type = 'Edit') {
  await focusApp();
  await ui(name, 'focus', { type });
  await win.call('paste', { text });
}

// LeGaJ and Edge start through Explorer, with the desktop's ordinary rights,
// as a user's double-click would: the runner's own shell is elevated, and
// Windows keeps windows of different integrity levels from messaging each
// other (single-instance activation, drag and drop). LeGaJ goes through a
// minimized .cmd so that its stderr, where a Go panic prints, is kept.
async function startApp() {
  const cmd = join(cfg.work, 'run-legaj.cmd');
  writeFileSync(cmd, `@"${cfg.exe}" 2>>"${join(cfg.out, 'legaj-stderr.txt')}"\r\n`);
  const lnk = join(cfg.work, 'run-legaj.lnk');
  await win.call('shortcut', { path: lnk, target: cmd, arguments: '', windowStyle: 7 });
  await win.call('shell', { path: lnk });
  const w = await legajWindow(60000);
  await win.call('move', { hwnd: w.Hwnd, x: 0, y: 0, w: 1024, h: 600 });
  await waitText(/Set up LeGaJ|Pipeline/, 60000);
}

async function quitApp() {
  const w = (await win.call('windows', { name: 'legaj' })).find((x) => x.Title === APP);
  if (w) await win.call('close', { hwnd: w.Hwnd });
  await until('LeGaJ to exit', async () => (await legajPids()).length === 0, 30000, 300);
}

async function nav(section) {
  await press(section);
  await sleep(600);
}

// ------------------------------------------------- the app's database

/** Runs a read-only query on LeGaJ's database with Python's sqlite3. */
function db(sql, ...params) {
  const py = [
    'import json, sqlite3, sys',
    'c = sqlite3.connect(sys.argv[1], timeout=10)',
    'c.row_factory = sqlite3.Row',
    'c.execute("PRAGMA query_only = 1")',
    'print(json.dumps([dict(r) for r in c.execute(sys.argv[2], json.loads(sys.argv[3]))]))',
  ].join('\n');
  const r = spawnSync('python', ['-I', '-c', py, DB, sql, JSON.stringify(params)], { encoding: 'utf8' });
  if (r.status !== 0) throw new Error(`db: ${(r.stderr || '').trim().split('\n').pop()}`);
  return JSON.parse(r.stdout);
}
const apps = () => db('SELECT id, company, role, location, url, stage, source FROM applications WHERE deleted_at IS NULL ORDER BY id');
const clipPort = () => Number(db("SELECT value FROM app_meta WHERE key = 'clip_port'")[0]?.value ?? 0);

// ------------------------------------------------------------------ Edge

let edge = null; // { cdp, tab, version }
const edgeProfile = join(cfg.work, 'edge-profile');
const EDGE_WINDOW = '*Microsoft*Edge*';

async function startEdge() {
  const lnk = join(cfg.work, 'edge-sweep.lnk');
  await win.call('shortcut', {
    path: lnk, target: EDGE,
    arguments: `--remote-debugging-port=${cfg.edgePort} "--user-data-dir=${edgeProfile}" --no-first-run --no-default-browser-check --window-position=0,${EDGE_TOP} --window-size=1024,${768 - EDGE_TOP} about:blank`,
  });
  await win.call('shell', { path: lnk });
  const ver = await until('Edge to start', async () => (await fetch(`http://127.0.0.1:${cfg.edgePort}/json/version`)).json(), 30000, 300);
  const cdp = await Devtools.connect(ver.webSocketDebuggerUrl);
  const { targetInfos } = await cdp.send('Target.getTargets');
  const page = targetInfos.find((t) => t.type === 'page');
  const { sessionId } = await cdp.send('Target.attachToTarget', { targetId: page.targetId, flatten: true });
  await cdp.send('Page.enable', {}, sessionId);
  await cdp.send('Runtime.enable', {}, sessionId);
  edge = { cdp, tab: sessionId, version: ver.Browser };
  await sleep(1500);
  return edge;
}

const edgeProcs = () => win.call('procs', { name: 'msedge.exe' });

async function stopEdge() {
  if (!edge) return;
  try {
    await edge.cdp.send('Browser.close');
  } catch {
    // closing drops the connection
  }
  edge = null;
  await until('Edge to exit', async () => !(await edgeProcs()).some((p) => p.cmd?.includes(edgeProfile)), 20000, 300);
}

async function navigate(url, settle = 3000) {
  await edge.cdp.send('Page.navigate', { url }, edge.tab);
  await until(`${url} to load`, () => evaluate(edge.cdp, `document.readyState === 'complete' && location.href !== 'about:blank'`, edge.tab), 45000, 300);
  await sleep(settle);
}

/** The sweep Edge window's own controls (not the page) whose name matches. */
const edgeUi = (name, type = '') => win.call('chrome', { window: EDGE_WINDOW, name, type, timeout: 5000 });
// A favorite on the bar, whatever control type Edge gives it.
async function bookmarks() {
  const ui = await edgeUi('*');
  const bar = ui.find((e) => e.name === 'Favorites bar' && e.w > 0);
  if (!bar) return [];
  return ui.filter((e) => e.name.startsWith('Clip to LeGaJ') && e.w > 0 && e.y >= bar.y && e.y < bar.y + bar.h);
}

/** Clicks the right-most "Clip to LeGaJ" favorite with the real mouse. */
async function clickBookmark() {
  const list = await until('the Clip to LeGaJ favorite', async () => {
    const b = await bookmarks();
    return b.length ? b : null;
  }, 10000, 500);
  const b = list.reduce((a, x) => (x.x > a.x ? x : a));
  const w = (await win.call('windows', { name: 'msedge' })).find((x) => /Microsoft.*Edge/.test(x.Title));
  if (w) await win.call('focus', { hwnd: w.Hwnd });
  await sleep(300);
  await win.call('click', { x: Math.round(b.x + b.w / 2), y: Math.round(b.y + b.h / 2) });
}

async function clickIn(cdp, sessionId, re) {
  const box = await until(`a control ${re}`, () => evaluate(cdp, `(() => {
    const re = new RegExp(${JSON.stringify(re.source)}, ${JSON.stringify(re.flags)});
    const el = [...document.querySelectorAll('button, a')].find((e) => re.test(e.innerText.trim()) && !e.disabled && e.offsetParent !== null);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
  })()`, sessionId), 15000, 200);
  const m = { x: box.x, y: box.y, button: 'left', clickCount: 1 };
  await cdp.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: box.x, y: box.y }, sessionId);
  await cdp.send('Input.dispatchMouseEvent', { type: 'mousePressed', ...m }, sessionId);
  await cdp.send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...m }, sessionId);
}

/** Clicks the favorite on the current page, saves in the popup, and returns what the popup said. */
async function clipCurrentPage(label, port) {
  // What the tab reports around the click (a blocked script, a refused
  // popup), so a missing popup can be explained.
  const said = [];
  const listen = (msg) => {
    if (msg.sessionId !== edge.tab) return;
    const p = msg.params ?? {};
    if (msg.method === 'Log.entryAdded') said.push(`${p.entry.level} ${p.entry.source}: ${p.entry.text}`);
    if (msg.method === 'Runtime.exceptionThrown') said.push(`exception: ${p.exceptionDetails?.exception?.description ?? p.exceptionDetails?.text}`);
    if (msg.method === 'Runtime.consoleAPICalled' && /error|warn/.test(p.type)) said.push(`console.${p.type}: ${(p.args ?? []).map((a) => a.value ?? a.description).join(' ')}`);
    if (msg.method === 'Page.windowOpen') said.push(`window.open ${p.url}`);
  };
  edge.cdp.listeners.add(listen);
  await edge.cdp.send('Log.enable', {}, edge.tab).catch(() => {});
  let popup;
  try {
    await clickBookmark();
    popup = await until('the clip popup', async () => {
      const { targetInfos } = await edge.cdp.send('Target.getTargets');
      return targetInfos.find((t) => t.type === 'page' && t.url.startsWith(`http://127.0.0.1:${port}/clip`));
    }, 15000, 250);
  } catch (err) {
    await shot(`no-popup-${label}`).catch(() => {});
    throw new Error(`${err.message}; the tab said: ${said.slice(-6).join(' | ').slice(0, 500) || 'nothing'}`);
  } finally {
    edge.cdp.listeners.delete(listen);
  }
  const { sessionId } = await edge.cdp.send('Target.attachToTarget', { targetId: popup.targetId, flatten: true });
  await edge.cdp.send('Runtime.enable', {}, sessionId);
  const state = () => evaluate(edge.cdp, `({ status: document.getElementById('status')?.textContent ?? '', kind: document.getElementById('status')?.className ?? '', save: !document.getElementById('save')?.hidden && !document.getElementById('save')?.disabled })`, sessionId);
  const first = await until('the popup to get the page', async () => {
    const s = await state();
    return s.save || /bad/.test(s.kind) ? s : null;
  }, 15000, 200);
  let final = first;
  if (first.save) {
    await clickIn(edge.cdp, sessionId, /^Save to LeGaJ$/);
    final = await until('the save to finish', async () => {
      const s = await state();
      return /good|bad/.test(s.kind) ? s : null;
    }, 30000, 200);
  }
  const { data } = await edge.cdp.send('Page.captureScreenshot', { format: 'png' }, sessionId).catch(() => ({ data: null }));
  if (data) writeFileSync(join(shots, `${String(++shotCount).padStart(2, '0')}-popup-${label}.png`), Buffer.from(data, 'base64'));
  const prompts = await edgeUi('*').catch(() => []);
  const lna = prompts.some((e) => /local network|devices on your/i.test(e.name));
  await edge.cdp.send('Target.closeTarget', { targetId: popup.targetId }).catch(() => {});
  return { ...final, lna };
}

/** Adds a favorite by writing Edge's Bookmarks file, as pasting an address would. Edge must be closed. */
function writeBookmark(url) {
  const now = String((Date.now() + 11644473600000) * 1000);
  const folder = (id, name, children) => ({ children, date_added: now, date_modified: now, id, name, type: 'folder' });
  const data = {
    roots: {
      bookmark_bar: folder('1', 'Favorites bar', [{ date_added: now, id: '5', name: 'Clip to LeGaJ', type: 'url', url }]),
      other: folder('2', 'Other favorites', []),
      synced: folder('3', 'Mobile favorites', []),
    },
    version: 1,
  };
  mkdirSync(join(edgeProfile, 'Default'), { recursive: true });
  writeFileSync(join(edgeProfile, 'Default', 'Bookmarks'), JSON.stringify(data, null, 2));
}

async function installByCopy(text) {
  await stopEdge();
  writeBookmark(text);
  await startEdge();
  return (await until('the pasted favorite', async () => ((await bookmarks()).length ? true : null), 10000, 500).catch(() => false)) === true;
}

// ------------------------------------------------------- local job pages

const ldOf = (title, company) => JSON.stringify({
  '@context': 'https://schema.org', '@type': 'JobPosting', title,
  hiringOrganization: { '@type': 'Organization', name: company },
  jobLocation: { '@type': 'Place', address: { '@type': 'PostalAddress', addressLocality: 'London' } },
  description: '<p>Own the weekly numbers and explain what they mean for the team\'s next decisions.</p>',
});
const jobPage = (title, company) => `<!doctype html><html><head><meta charset="utf-8"><title>${title} at ${company}</title>
<script type="application/ld+json">${ldOf(title, company)}</script>
</head><body><main><h1>${title}</h1><p>${company} is hiring. Own the weekly numbers and explain what they mean.</p></main></body></html>`;
const pages = createHttpServer((req, res) => {
  const u = new URL(req.url, 'http://x');
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(jobPage(u.searchParams.get('t') ?? 'Data Analyst', u.searchParams.get('c') ?? 'Babbage and Co'));
});
await new Promise((r) => pages.listen(8765, '127.0.0.1', r));
const localJob = (n, title = 'Data Analyst', company = 'Babbage and Co') => `http://127.0.0.1:8765/jobs/${n}?t=${encodeURIComponent(title)}&c=${encodeURIComponent(company)}`;

/** Sends a clip to LeGaJ's clip server the way the popup does (for bulk rows). */
async function apiClip(port, n, title, company) {
  const origin = `http://127.0.0.1:${port}`;
  const html = await (await fetch(`${origin}/clip`)).text();
  const session = /legaj-session" content="([0-9a-f]{32})"/.exec(html)[1];
  const page = {
    type: 'legaj-clip', v: 1, url: `http://127.0.0.1:8765/jobs/bulk-${n}`, title: `${title} at ${company}`,
    jsonld: [ldOf(title, company)], meta: {}, h1: [title], selection: '',
    text: `${title}. ${company} is hiring. Own the weekly numbers and explain what they mean for the team.`,
  };
  const res = await fetch(`${origin}/api/clips`, { method: 'POST', headers: { 'Content-Type': 'application/json', Origin: origin }, body: JSON.stringify({ session, page }) });
  if (!res.ok) throw new Error(`clip ${n}: ${res.status} ${await res.text()}`);
}

// ---------------------------------------------------- live job postings

async function liveJobs() {
  const jobs = [];
  const tryEach = async (site, fn) => {
    try {
      jobs.push({ site, url: await fn() });
    } catch (err) {
      jobs.push({ site, url: null, why: err.message });
    }
  };
  const json = async (u, init) => {
    const r = await fetch(u, { ...init, signal: AbortSignal.timeout(20000) });
    if (!r.ok) throw new Error(`${u} answered ${r.status}`);
    return r.json();
  };
  await tryEach('Greenhouse', async () => {
    for (const board of ['anthropic', 'discord', 'figma', 'cloudflare', 'dropbox']) {
      const d = await json(`https://boards-api.greenhouse.io/v1/boards/${board}/jobs`).catch(() => null);
      const j = d?.jobs?.find((x) => /greenhouse\.io/.test(x.absolute_url));
      if (j) return j.absolute_url;
    }
    throw new Error('no board with a hosted job page');
  });
  await tryEach('Lever', async () => {
    for (const co of ['spotify', 'palantir', 'plaid', 'zoox']) {
      const d = await json(`https://api.lever.co/v0/postings/${co}?mode=json&limit=1`).catch(() => null);
      if (d?.[0]?.hostedUrl) return d[0].hostedUrl;
    }
    throw new Error('no Lever board answered');
  });
  await tryEach('Ashby', async () => {
    for (const org of ['openai', 'notion', 'ramp', 'linear']) {
      const d = await json(`https://api.ashbyhq.com/posting-api/job-board/${org}`).catch(() => null);
      if (d?.jobs?.[0]?.jobUrl) return d.jobs[0].jobUrl;
    }
    throw new Error('no Ashby board answered');
  });
  await tryEach('Workday', async () => {
    const base = 'https://nvidia.wd5.myworkdayjobs.com';
    const d = await json(`${base}/wday/cxs/nvidia/NVIDIAExternalCareerSite/jobs`, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ appliedFacets: {}, limit: 1, offset: 0, searchText: '' }),
    });
    return `${base}/en-US/NVIDIAExternalCareerSite${d.jobPostings[0].externalPath}`;
  });
  await tryEach('LinkedIn (guest)', async () => {
    const r = await fetch('https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search?keywords=analyst&location=United%20States&start=0', { signal: AbortSignal.timeout(20000) });
    const m = /urn:li:jobPosting:(\d+)/.exec(await r.text());
    if (!m) throw new Error(`no job id in LinkedIn's guest search (status ${r.status})`);
    return `https://www.linkedin.com/jobs/view/${m[1]}/`;
  });
  await tryEach('Indeed', async () => {
    const r = await fetch('https://www.indeed.com/jobs?q=analyst&l=Remote', { signal: AbortSignal.timeout(20000), headers: { 'User-Agent': 'Mozilla/5.0' } });
    const m = /data-jk="([0-9a-f]{16})"/.exec(await r.text());
    if (!m) throw new Error(`no job id in Indeed's search (status ${r.status}); its bot check likely blocked the runner`);
    return `https://www.indeed.com/viewjob?jk=${m[1]}`;
  });
  return jobs;
}

// ---------------------------------------------------------------- utils

const sha = (path) => createHash('sha256').update(readFileSync(path)).digest('hex');

function findFiles(dir, re, out = []) {
  if (!existsSync(dir)) return out;
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) findFiles(p, re, out);
    else if (re.test(name)) out.push({ path: p, mtime: st.mtimeMs });
  }
  return out;
}

/** Waits for a new msedge.exe whose command line contains needle. */
async function edgeOpened(needle, before) {
  const seen = new Set(before.map((p) => p.pid));
  return until(`Edge to open ${needle}`, async () => (await edgeProcs()).find((p) => !seen.has(p.pid) && p.cmd?.toLowerCase().includes(needle.toLowerCase())), 15000, 250);
}

/** Closes every Edge window except the sweep's own, so the next open starts a fresh process. */
async function closeDefaultEdge() {
  const pids = (await edgeProcs()).filter((p) => !p.cmd?.includes(edgeProfile)).map((p) => p.pid);
  if (pids.length) await win.call('kill', { pids });
  await sleep(1000);
}

const CONSOLE = /^(cmd|conhost|powershell|pwsh|cscript|wscript)\.exe$/i;
/** Console processes started by LeGaJ (or its children) since the last call. */
async function consoleChildren() {
  const starts = await win.call('starts');
  const family = new Set(await legajPids());
  for (const s of starts) if (family.has(s.ppid)) family.add(s.pid);
  return starts.filter((s) => family.has(s.ppid) && CONSOLE.test(s.name));
}
const consoleNote = (list) => `console process: ${list.map((x) => x.name).join(', ')}`;

/** Clicks a row's control in Pipeline and returns the Edge process it opened. */
async function openFromRow(row, name, needle) {
  await closeDefaultEdge();
  await consoleChildren();
  const before = await edgeProcs();
  await ui(name, 'invoke', { row, type: 'Button' });
  return edgeOpened(needle, before);
}

/** Keeps LeGaJ's own logs for the artifact (the last uninstall deletes them). */
function saveAppLogs() {
  const dir = join(cfg.out, 'app-logs');
  mkdirSync(dir, { recursive: true });
  for (const f of findFiles(join(DATA, 'logs'), /./)) writeFileSync(join(dir, basename(f.path)), readFileSync(f.path));
}

/** Ends any uninstaller left from an earlier check (it runs as a _iu*.tmp copy). */
async function stopUninstallers() {
  await win.call('kill-name', { name: '_iu*' });
  await win.call('kill-name', { name: 'unins000' });
  await sleep(500);
}

/** Which tables of WebView2's "Web Data" (Chromium's autofill store) hold the sentinels. */
function webDataReport() {
  const file = join(WEBVIEW_DATA, 'EBWebView', 'Default', 'Web Data');
  if (!existsSync(file)) return 'no Web Data file';
  const py = [
    'import json, shutil, sqlite3, sys, tempfile, os',
    'tmp = os.path.join(tempfile.mkdtemp(), "wd")',
    'shutil.copyfile(sys.argv[1], tmp)',
    'c = sqlite3.connect(tmp)',
    'needles = json.loads(sys.argv[2])',
    'out = []',
    'for (t,) in c.execute("SELECT name FROM sqlite_master WHERE type = \'table\'"):',
    '    cols = [r[1] for r in c.execute(f"PRAGMA table_info([{t}])")]',
    '    for col in cols:',
    '        for label, n in needles.items():',
    '            q = f"SELECT count(*) FROM [{t}] WHERE CAST([{col}] AS TEXT) LIKE ?"',
    '            k = c.execute(q, (f"%{n}%",)).fetchone()[0]',
    '            if k: out.append(f"{label} in {t}.{col} ({k} rows)")',
    'print("; ".join(out) or "no table holds them as text")',
  ].join('\n');
  const s = cfg.sentinel;
  const r = spawnSync('python', ['-I', '-c', py, file, JSON.stringify({ email: s.email, phone: s.phone, 'resume line': s.line, 'API key': s.key })], { encoding: 'utf8' });
  return r.status === 0 ? r.stdout.trim() : `could not read it: ${(r.stderr || '').trim().split('\n').pop()}`;
}

async function install(extra = []) {
  const log = join(cfg.out, `install-${Date.now()}.log`);
  const p = spawn(cfg.setup, ['/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', `/LOG=${log}`, ...extra], { stdio: 'ignore' });
  const code = await new Promise((r) => p.on('exit', r));
  return { code, log: readFileSync(log, 'utf8') };
}

/** Waits for a window or owned dialog whose title matches and whose text passes re. */
async function dialog(titleLike, re, ms = 20000) {
  return until(`a dialog like ${titleLike} saying ${re}`, async () => (await win.call('dialogs', { window: titleLike })).find((d) => re.test(`${d.title} ${d.text}`)), ms, 300);
}
const pressIn = (d, button) => win.call('press', { hwnd: d.hwnd, button });

/** Turns "Start LeGaJ with Windows" on in Settings and saves. */
async function startWithWindows() {
  await nav('Settings');
  const box = await ui('Start LeGaJ with Windows*', 'find', { type: 'CheckBox' });
  if (box.toggle !== 'On') {
    await ui('Start LeGaJ with Windows*', 'toggle', { type: 'CheckBox' });
    await press('Save settings');
  }
  return until('the Run value', () => win.call('regvalue', { key: RUN_KEY, name: 'LeGaJ' }), 10000, 300);
}

// ================================================================= main

async function mainPhase() {
  const s = cfg.sentinel;
  await win.call('watch-starts');

  // ---- Installer (R38)
  await check('R38', 'Silent install: per user, Start menu entry, no WebView2 step when it is present', async () => {
    const r = await install();
    const problems = [];
    if (r.code !== 0) problems.push(`exit ${r.code}`);
    if (!existsSync(cfg.exe)) problems.push('no exe in %LOCALAPPDATA%\\Programs\\LeGaJ');
    if (!existsSync(START_MENU)) problems.push('no Start menu entry');
    if (!/Administrative install mode: No/.test(r.log)) problems.push('not a per-user install');
    if (/WebView2/i.test(r.log)) problems.push('a WebView2 step ran');
    return problems.length ? fail(problems.join('; ')) : pass('installed per user; no WebView2 step');
  });

  // ---- Onboarding with sentinel values
  await check('item45', 'First start on a clean machine shows the setup wizard', async () => {
    await startApp();
    await waitText(/Set up LeGaJ/, 30000);
    await shot('wizard');
    return pass('the wizard opened');
  });

  await check('R4', 'Saving a key in the wizard stores LeGaJ:gemini-api-key in Credential Manager', async () => {
    await fill('Gemini API key', s.key);
    await press('Save and test');
    await waitText(/Key works/, 30000);
    const r = await win.call('run', { file: 'cmdkey.exe', args: ['/list'] });
    return /LeGaJ:gemini-api-key/.test(r.output) ? pass('key tested against the stand-in Gemini, then saved') : fail(r.output.slice(0, 300));
  });

  await check('item45', 'Resume upload through the Windows file dialog, then the profile review', async () => {
    await press('Next');
    await press('Choose resume file*');
    await win.call('filedialog', { window: '*Choose your resume*', path: cfg.resume1 });
    await waitText(/Check your profile/, 30000);
    const email = await ui('Email', 'find', { type: 'Edit' });
    if (email.value !== s.email) return fail(`the Email field holds "${email.value}"`);
    await press('Save profile');
    await waitText(/Clip jobs from your browser/);
    return pass('resume read; the review showed the parsed email; profile saved');
  });

  // ---- The clipper install page: drag and copy (R18)
  await startEdge();
  let bookmarkInstalled = false;
  let dragWorked = false;
  await check('R18', 'Drag "Clip to LeGaJ" from the app onto Edge\'s favorites bar', async () => {
    await focusApp();
    let link = await ui('Clip to LeGaJ', 'scroll', { type: 'Hyperlink' });
    for (let i = 0; i < 6 && link.y + link.h / 2 > EDGE_TOP - 30; i++) {
      await win.call('wheel', { x: 512, y: 250, delta: -120 });
      await sleep(400);
      link = await ui('Clip to LeGaJ', 'find', { type: 'Hyperlink' });
    }
    const from = { x: Math.round(link.x + link.w / 2), y: Math.round(link.y + link.h / 2) };
    const ui0 = await edgeUi('*');
    writeFileSync(join(cfg.out, 'edge-ui.json'), JSON.stringify(ui0, null, 1));
    // The empty right end of the favorites bar: a drop on one of its
    // buttons ("Import favorites") doesn't make a favorite.
    const bar = ui0.filter((e) => e.name === 'Favorites bar' && e.w > 0);
    const to = bar.length ? { x: Math.round(bar[0].x + bar[0].w - 120), y: Math.round(bar[0].y + bar[0].h / 2) } : { x: 880, y: EDGE_TOP + 96 };
    await shot('before-drag');
    await win.call('down', { x: from.x, y: from.y });
    await win.call('moveto', { x1: from.x + 12, y1: from.y + 6, x2: to.x, y2: to.y, steps: 40 });
    await sleep(800);
    await shot('mid-drag');
    await win.call('up', { x: to.x, y: to.y });
    await sleep(1500);
    await shot('after-drag');
    dragWorked = (await bookmarks()).length > 0;
    bookmarkInstalled = dragWorked;
    return dragWorked
      ? pass(`dragged from ${from.x},${from.y} to ${to.x},${to.y}; Edge made the favorite`)
      : fail(`no favorite after dragging from ${from.x},${from.y} to ${to.x},${to.y}; favorites-bar controls: ${JSON.stringify(bar).slice(0, 200)}`);
  });

  let copied = '';
  await check('R18', '"Copy the clipper" puts the bookmarklet, with this run\'s port, on the clipboard', async () => {
    await press('Copy the clipper');
    copied = await until('the clipboard', async () => {
      const t = await win.call('clipboard');
      return t?.startsWith('javascript:') ? t : null;
    }, 5000, 250);
    const port = clipPort();
    const decoded = decodeURIComponent(copied);
    return decoded.includes(`127.0.0.1:${port}`) ? pass(`${copied.length} characters; port ${port}`) : fail(`no address for port ${port} in the copy`);
  });

  if (!dragWorked && copied) {
    bookmarkInstalled = await installByCopy(copied);
    record('R18', 'Fallback the page offers: the copied clipper, pasted as a favorite\'s address, shows on the bar', bookmarkInstalled ? 'PASS' : 'FAIL', bookmarkInstalled ? 'favorite made from the clipboard text' : 'Edge shows no favorite');
  }

  await check('item45', 'Finish the wizard', async () => {
    await focusApp();
    await press('Finish');
    await waitNoText(/Set up LeGaJ/);
    await shot('after-wizard');
  });

  // ---- First clip (R18)
  let port = clipPort();
  await check('R18', 'A real favorite click on a job page, then Save: the job is in Pipeline at once and "Install the clipper" leaves the checklist', async () => {
    if (!bookmarkInstalled) return skip('no favorite to click');
    const before = /Install the clipper/.test(await appText());
    await navigate(localJob(1, 'Data Analyst', 'Babbage and Co'));
    const r = await clipCurrentPage('local', port);
    if (!/good/.test(r.kind)) return fail(`popup says "${r.status}"`);
    const row = apps().find((x) => x.url.includes('127.0.0.1:8765/jobs/1'));
    const problems = [];
    if (!row) problems.push('no row with the page URL');
    if (row && row.source !== 'clip') problems.push(`row source is ${row.source}`);
    await nav('Pipeline');
    await waitText(/Babbage and Co/, 10000).catch(() => problems.push('the row is not shown in Pipeline'));
    if (!before) problems.push('"Install the clipper" was not on the checklist before the clip');
    await waitNoText(/Install the clipper/, 10000).catch(() => problems.push('"Install the clipper" is still on the checklist'));
    await shot('after-first-clip');
    return problems.length ? fail(problems.join('; ')) : pass(`popup: "${r.status}"; row: ${row.company} / ${row.role} / ${row.location}${r.lna ? '; LOCAL NETWORK PROMPT SEEN' : ''}`);
  });

  // ---- The 6 sites in Edge (item 45)
  if (bookmarkInstalled) {
    for (const job of await liveJobs()) {
      await check('item45', `Clip on ${job.site} in Edge ${edge?.version ?? ''}`, async () => {
        if (!job.url) return skip(job.why);
        const before = new Set(apps().map((x) => x.id));
        await navigate(job.url, 6000);
        const href = await evaluate(edge.cdp, 'location.href', edge.tab);
        const r = await clipCurrentPage(job.site.replace(/\W+/g, '-'), port);
        await sleep(1500);
        const row = apps().find((x) => !before.has(x.id));
        const fields = row ? `company "${row.company}", role "${row.role}", location "${row.location}"` : 'no new row';
        const note = `${job.url}${href !== job.url ? ` (landed on ${href})` : ''}: popup "${r.status}"; ${fields}${r.lna ? '; LOCAL NETWORK PROMPT SEEN' : ''}`;
        const ok = /good/.test(r.kind) && row;
        if (job.site.startsWith('LinkedIn')) return { status: ok ? 'PASS' : 'FAIL', detail: `logged out (no account on the runner): ${note}` };
        return ok ? pass(note) : fail(note);
      });
    }
  }

  // ---- Features with sentinel values (item 45)
  const NW = 'Select Northwind Analytics, Operations Analyst';
  await check('item45', 'Paste a job in Pipeline', async () => {
    await nav('Pipeline');
    await press('Paste job');
    await fill('Job URL', 'https://jobs.example.org/sweep/analyst');
    await fill('Company', 'Northwind Analytics');
    await fill('Role', 'Operations Analyst');
    await fill('Location', 'Remote');
    await fill('Job description*', 'Northwind Analytics is hiring an Operations Analyst to own weekly reporting, find the drivers behind the numbers, and help teams decide what to do next.', 'Edit');
    await press('Add to pipeline');
    const row = await until('the pasted row', () => apps().find((x) => x.company === 'Northwind Analytics'), 10000, 300);
    return pass(`row ${row.id}, ${row.stage}`);
  });

  await check('item45', 'Generate a letter from the row, then approve it: a PDF is written', async () => {
    await press('Generate letter for Northwind Analytics');
    const appId = apps().find((x) => x.company === 'Northwind Analytics').id;
    await until('the draft', () => db("SELECT 1 FROM documents WHERE application_id = ? AND status = 'draft'", appId).length > 0, 60000, 500);
    await waitText(/Approve/, 15000);
    await shot('review');
    await press('Approve');
    const doc = await until('the approved letter', () => db("SELECT file_path FROM documents WHERE application_id = ? AND status = 'approved'", appId)[0], 30000, 500);
    const pdf = join(DATA, doc.file_path);
    return existsSync(pdf) ? pass(doc.file_path) : fail(`no file at ${doc.file_path}`);
  });

  await check('item45', 'Draft form answers and approve one', async () => {
    await nav('Pipeline');
    await press('Answers for Northwind Analytics');
    await press('Generate answers');
    await press('Approve option 1', { timeout: 60000 });
    const a = await until('the approved answer', () => db("SELECT status, length(answer) AS n FROM answers WHERE status = 'approved'")[0], 15000, 500);
    await press('Close').catch(() => {});
    return pass(`approved an answer of ${a.n} characters`);
  });

  // ---- Opening links and files (R33, R10)
  await check('R33', 'Posting opens the job page in the default browser (Edge), with no console window', async () => {
    await nav('Pipeline');
    const p = await openFromRow(NW, 'Posting', 'jobs.example.org/sweep/analyst');
    const consoles = await consoleChildren();
    return consoles.length ? fail(consoleNote(consoles)) : pass(p.cmd.slice(0, 200));
  });
  await check('R33', 'The row\'s Letter opens the approved PDF in the default viewer', async () => {
    const p = await openFromRow(NW, 'Open letter for Northwind Analytics', '.pdf');
    const consoles = await consoleChildren();
    return consoles.length ? fail(consoleNote(consoles)) : pass(p.cmd.replace(DATA, '%LOCALAPPDATA%\\LeGaJ').slice(0, 220));
  });
  await check('R10', 'After Applied, a second resume upload leaves the row\'s Resume on the first file', async () => {
    await press('Paste job');
    await fill('Job URL', 'https://jobs.example.org/sweep/applied');
    await fill('Company', 'Fabrikam Research');
    await fill('Role', 'Research Analyst');
    await fill('Job description*', 'Fabrikam Research is hiring a research analyst to turn survey data into clear weekly findings for the product teams.', 'Edit');
    await press('Add to pipeline');
    const row = await until('the pasted row', () => apps().find((x) => x.company === 'Fabrikam Research'), 10000, 300);
    const FAB = 'Select Fabrikam Research, Research Analyst';
    await ui(FAB, 'toggle', { type: 'CheckBox' });
    await focusApp();
    await ui('Move selected to', 'focus', { type: 'ComboBox' });
    await win.call('type', { text: 'Applied' });
    await press('Move');
    await until('the row to be Applied', () => apps().find((x) => x.id === row.id)?.stage === 'applied', 10000, 300);
    await nav('Profile');
    await press('Upload a new version');
    await win.call('filedialog', { window: '*Choose your resume*', path: cfg.resume2 });
    await until('the second resume', () => db('SELECT count(*) AS n FROM resume_files')[0].n >= 2, 20000, 500);
    await nav('Pipeline');
    const p = await openFromRow(FAB, 'Resume', '.pdf');
    const path = (p.cmd.split('--single-argument')[1] ?? '').trim().replace(/^"|"$/g, '');
    const consoles = await consoleChildren();
    if (consoles.length) return fail(consoleNote(consoles));
    if (!path || !existsSync(path)) return fail(`opened ${p.cmd}`);
    const h = sha(path);
    return h === sha(cfg.resume1) ? pass(`opened ${path.replace(DATA, '%LOCALAPPDATA%\\LeGaJ')}: the first file, byte for byte`) : fail(h === sha(cfg.resume2) ? 'opened the second resume' : `opened an unknown file ${path}`);
  });
  await closeDefaultEdge();

  // ---- Focus (R6)
  await check('R6', 'Generating 10 letters while another app has focus: LeGaJ never takes focus, no console window', async () => {
    for (let i = 1; i <= 10; i++) await apiClip(port, i, `Analyst ${i}`, 'Contoso Data');
    const ids = await until('10 bulk rows', () => {
      const r = apps().filter((x) => x.company === 'Contoso Data');
      return r.length >= 10 ? r.map((x) => x.id) : null;
    }, 15000, 500);
    await nav('Pipeline');
    await waitText(/Analyst 10/, 10000);
    await ui('Select all shown', 'toggle', { type: 'CheckBox' });
    await ui('Generate letters', 'find', { type: 'Button', enabled: true });
    await win.call('shell', { path: 'C:\\Windows\\System32\\notepad.exe' });
    const pad = await until('Notepad', async () => (await win.call('windows', { name: 'notepad' }))[0], 10000, 250);
    await win.call('focus', { hwnd: pad.Hwnd });
    await sleep(800);
    const fg0 = await win.call('foreground');
    if (fg0.Pid !== pad.Pid) return fail(`Notepad never got focus (foreground: ${fg0.Title})`);
    await consoleChildren();
    const app = new Set(await legajPids());
    // UI Automation clicks the button without bringing LeGaJ forward.
    await press('Generate letters');
    // A UI Automation click can bring LeGaJ forward itself, which says
    // nothing about the app; focus goes back to Notepad, then the sampling
    // covers the generation (the stand-in Gemini takes 1.5 s per letter).
    const byClick = app.has((await win.call('foreground')).Pid);
    await win.call('focus', { hwnd: pad.Hwnd });
    await sleep(300);
    const stolen = [];
    let samples = 0;
    const end = Date.now() + 120000;
    const drafts = () => db(`SELECT count(DISTINCT application_id) AS n FROM documents WHERE status = 'draft' AND application_id IN (${ids.join(',')})`)[0].n;
    let settled = 0;
    while (Date.now() < end) {
      const fg = await win.call('foreground');
      samples++;
      if (app.has(fg.Pid)) stolen.push(fg.Title);
      if (samples % 10 === 0 && (settled = drafts()) >= ids.length) break;
      await sleep(50);
    }
    for (let i = 0; i < 30; i++) {
      const fg = await win.call('foreground');
      samples++;
      if (app.has(fg.Pid)) stolen.push(fg.Title);
      await sleep(100);
    }
    settled = drafts();
    const consoles = await consoleChildren();
    await win.call('kill', { pids: [pad.Pid] });
    const problems = [];
    if (stolen.length) problems.push(`LeGaJ had focus in ${stolen.length} of ${samples} samples`);
    if (consoles.length) problems.push(consoleNote(consoles));
    if (settled < ids.length) problems.push(`only ${settled} of ${ids.length} drafts`);
    return problems.length ? fail(problems.join('; ')) : pass(`${settled} drafts; Notepad kept focus in all ${samples} samples${byClick ? ' (the UI Automation click itself had brought LeGaJ forward)' : ''}`);
  });

  // ---- Gemini wire format (R35)
  await check('R35', 'Every Gemini call carries the key in the x-goog-api-key header only', async () => {
    const file = join(cfg.out, 'gemini-requests.jsonl');
    const lines = existsSync(file) ? readFileSync(file, 'utf8').trim().split('\n').filter(Boolean).map((l) => JSON.parse(l)) : [];
    if (!lines.length) return skip('LeGaJ made no Gemini calls in this run');
    const bad = lines.filter((l) => !l.keyInHeader || l.keyInUrl || l.keyInBody);
    const tasks = [...new Set(lines.map((l) => l.task).filter(Boolean))];
    return bad.length ? fail(`${bad.length} of ${lines.length} calls: ${JSON.stringify(bad[0])}`) : pass(`${lines.length} calls (${tasks.join('; ')})`);
  });

  // ---- Single instance (R42)
  await check('R42', 'Launching again from the Start menu shows no second window, and restores and focuses the first', async () => {
    const w = await legajWindow();
    await win.call('minimize', { hwnd: w.Hwnd });
    await sleep(800);
    await win.call('shell', { path: START_MENU });
    await sleep(4000);
    const wins = (await win.call('windows', { name: 'legaj' })).filter((x) => x.Title === APP);
    const fg = await win.call('foreground');
    const pids = await legajPids();
    const problems = [];
    if (wins.length !== 1) problems.push(`${wins.length} LeGaJ windows`);
    if (wins[0]?.Iconic) problems.push('still minimized');
    if (pids.length !== 1) problems.push(`${pids.length} legaj processes`);
    if (wins[0] && fg.Hwnd !== wins[0].Hwnd) problems.push(`the foreground window is "${fg.Title}"`);
    await shot('second-launch');
    return problems.length ? fail(problems.join('; ')) : pass('one window, restored and in front');
  });
  await check('R42', 'Two launches in quick succession open one window', async () => {
    await quitApp();
    await win.call('shell', { path: START_MENU });
    await win.call('shell', { path: START_MENU });
    let most = 0;
    for (let i = 0; i < 60; i++) {
      most = Math.max(most, (await win.call('windows', { name: 'legaj' })).filter((x) => x.Title === APP).length);
      await sleep(150);
    }
    const pids = await legajPids();
    return most === 1 && pids.length === 1 ? pass('one window throughout; one process left') : fail(`up to ${most} windows; ${pids.length} processes`);
  });

  // ---- Reinstall banner (R18)
  await check('R18', 'Port taken: the banner shows, Reinstall opens the install page, a clip with the new favorite clears it without a restart', async () => {
    if (!bookmarkInstalled) return skip('no favorite to reinstall');
    const old = clipPort();
    await quitApp();
    const hold = createTcpServer();
    await new Promise((r, j) => hold.once('error', j).listen(old, '127.0.0.1', r));
    try {
      await startApp();
      await waitText(/no longer reaches LeGaJ/, 20000);
      const now = clipPort();
      if (now === old) return fail(`the app kept port ${old}`);
      await shot('banner');
      await press('Reinstall bookmarklet');
      await ui('Clip to LeGaJ', 'find', { type: 'Hyperlink' });
      await press('Copy the clipper');
      const fresh = await until('the clipboard', async () => {
        const t = await win.call('clipboard');
        return t && decodeURIComponent(t).includes(`127.0.0.1:${now}`) ? t : null;
      }, 5000, 250);
      if (!(await installByCopy(fresh))) return fail('the new favorite did not show');
      await navigate(localJob(2, 'Reporting Analyst', 'Babbage and Co'));
      const r = await clipCurrentPage('after-reinstall', now);
      if (!/good/.test(r.kind)) return fail(`popup says "${r.status}"`);
      await waitNoText(/no longer reaches LeGaJ/, 10000);
      port = now;
      return pass(`port ${old} held; LeGaJ moved to ${now}; the banner cleared after one clip, without a restart`);
    } finally {
      hold.close();
    }
  });
  await stopEdge();

  // ---- Start with Windows, then the sentinel scan (item 45)
  await check('R37', '"Start LeGaJ with Windows" writes the Run value', async () => {
    const v = await startWithWindows();
    return pass(v);
  });
  const rows = apps().length;
  await quitApp();
  await check('item45', 'scan-sentinels.ps1 finds no key, email, phone or resume line where they must not be', async () => {
    const r = await win.call('run', {
      file: 'powershell.exe',
      args: ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', join(cfg.repo, 'scripts', 'scan-sentinels.ps1'), '-Key', s.key, '-Email', s.email, '-Phone', s.phone, '-ResumeLine', s.line],
    });
    writeFileSync(join(cfg.out, 'scan-sentinels.txt'), r.output);
    if (r.code === 0 && /nothing found/.test(r.output)) return pass('nothing found in the data folder, the logs or the WebView2 folder');
    const hits = r.output.split('\n').filter((l) => /^\s+\S.*: /.test(l)).map((l) => l.trim().replace(/C:\\Users\\[^\\]+\\AppData\\(Local|Roaming)/, '%$1%'));
    const webData = r.output.includes('Web Data') ? `; Web Data: ${webDataReport()}` : '';
    return fail(`${hits.join(' | ') || r.output.slice(-300)}${webData}`);
  });
  await check('item45', 'The database holds the sentinel profile, so the scan looked where the data is', async () => {
    const hit = db("SELECT 1 FROM profile WHERE data LIKE ?", `%${s.email}%`).length > 0;
    const logs = findFiles(join(DATA, 'logs'), /./).length;
    return hit ? pass(`profile holds the email, as it should; ${logs} log file(s) scanned`) : fail('the sentinel email is not in the profile');
  });

  // ---- Installer with LeGaJ running (R38)
  await startApp();
  const PROMPT = /currently running|close (all|the) (instances|application)/i;
  await check('R38', 'Setup started while LeGaJ runs asks to close it first', async () => {
    const p = spawn(cfg.setup, [], { stdio: 'ignore' });
    try {
      const d = await dialog('*Setup*', /running|close|Welcome|Ready|Select/i, 30000);
      await shot('setup-while-running');
      writeFileSync(join(cfg.out, 'setup-while-running.txt'), JSON.stringify(d, null, 1));
      for (const b of ['Cancel', 'No', 'Exit Setup']) if (d.buttons.includes(b)) { await pressIn(d, b); break; }
      await sleep(1500);
      const confirm = (await win.call('dialogs', { window: '*Setup*' })).find((x) => /exit setup/i.test(x.text));
      if (confirm) await pressIn(confirm, 'Yes');
      return PROMPT.test(d.text) ? pass(d.text.slice(0, 220)) : fail(`setup showed: ${d.title}: ${d.text.slice(0, 300)}`);
    } finally {
      p.kill();
    }
  });
  await check('R38', 'The uninstaller started while LeGaJ runs asks to close it first, and removes nothing', async () => {
    await stopUninstallers();
    spawn(UNINSTALLER, [], { stdio: 'ignore' });
    let d = await dialog('*Uninstall*', /running|close|sure/i, 30000);
    let text = d.text;
    if (/Are you sure/i.test(d.text)) {
      await pressIn(d, 'Yes');
      d = await dialog('*Uninstall*', /running|close|removed|Keep/i, 30000);
      text += ' || ' + d.text;
    }
    await shot('uninstall-while-running');
    const asks = PROMPT.test(d.text);
    for (const b of ['Cancel', 'No', 'OK']) if (d.buttons.includes(b)) { await pressIn(d, b); break; }
    await sleep(3000);
    const left = (await win.call('dialogs', { window: '*Uninstall*' })).length;
    await stopUninstallers();
    if (left) text += ` || (the uninstaller still showed ${left} window(s) after Cancel)`;
    const intact = existsSync(cfg.exe) && existsSync(DB);
    return asks && intact ? pass(text.slice(0, 250)) : fail(`uninstaller showed: ${text.slice(0, 300)}; app and data intact: ${intact}`);
  });
  await quitApp();

  // ---- Uninstall: keep, reinstall, uninstall: delete (R38)
  // unins000.exe copies itself to a temp folder and exits at once, so the
  // dialogs, not the process, tell when it's done.
  async function uninstall(keep) {
    await stopUninstallers();
    spawn(UNINSTALLER, [], { stdio: 'ignore' });
    const seen = [];
    for (let i = 0; i < 5; i++) {
      const d = await dialog('*Uninstall*', /sure|Keep your applications|removed|running/i, 60000).catch(() => null);
      if (!d) break;
      seen.push(d.text);
      if (/Are you sure/i.test(d.text)) await pressIn(d, 'Yes');
      else if (/Keep your applications/i.test(d.text)) await pressIn(d, keep ? 'Yes' : 'No');
      else if (/removed/i.test(d.text)) {
        await pressIn(d, 'OK');
        break;
      } else break;
      await until('the dialog to close', async () => !(await win.call('dialogs', { window: '*Uninstall*' })).some((x) => x.hwnd === d.hwnd), 10000, 300).catch(() => {});
    }
    await sleep(2000);
    return seen;
  }
  await check('R38', 'Uninstall, answer Yes to keep: the data stays, the Run value goes; reinstalling brings the data back', async () => {
    const seen = await uninstall(true);
    const problems = [];
    if (!seen.some((t) => /Keep your applications/i.test(t))) problems.push(`no keep question (saw: ${seen.join(' || ').slice(0, 200)})`);
    if (!existsSync(DB)) problems.push('the database is gone');
    if (await win.call('regvalue', { key: RUN_KEY, name: 'LeGaJ' })) problems.push('the Run value is still there');
    if (existsSync(cfg.exe)) problems.push('legaj.exe is still installed');
    const r = await install();
    if (r.code !== 0) problems.push(`reinstall exit ${r.code}`);
    await startApp();
    const back = apps().length;
    if (back !== rows) problems.push(`${back} applications after reinstall, ${rows} before`);
    await waitText(/Northwind Analytics/, 10000).catch(() => problems.push('Pipeline does not show the old rows'));
    return problems.length ? fail(problems.join('; ')) : pass(`data kept; all ${back} applications back after reinstall`);
  });
  saveAppLogs();
  await check('R38', 'Uninstall, answer No: the data folder, WebView2 data, the saved key and the Run value are gone', async () => {
    await startWithWindows();
    await quitApp();
    const seen = await uninstall(false);
    const problems = [];
    if (!seen.some((t) => /Keep your applications/i.test(t))) problems.push('no keep question');
    if (existsSync(DATA)) problems.push('%LOCALAPPDATA%\\LeGaJ is still there');
    if (existsSync(WEBVIEW_DATA)) problems.push('%APPDATA%\\legaj.exe is still there');
    const keys = await win.call('run', { file: 'cmdkey.exe', args: ['/list'] });
    if (/LeGaJ:gemini-api-key/.test(keys.output)) problems.push('the key is still in Credential Manager');
    if (await win.call('regvalue', { key: RUN_KEY, name: 'LeGaJ' })) problems.push('the Run value is still there');
    return problems.length ? fail(problems.join('; ')) : pass('all removed');
  });
}

// ------------------------------------------------- standard user (R38)

async function standardPhase() {
  await check('R38', 'A standard (non-admin) user installs with no elevation, to their own %LOCALAPPDATA%\\Programs\\LeGaJ', async () => {
    const r = await win.call('run', { file: 'powershell.exe', args: ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', join(cfg.repo, 'scripts', 'sweep', 'standard-user.ps1'), '-Setup', cfg.setup, '-Out', cfg.out] });
    writeFileSync(join(cfg.out, 'standard-user.txt'), r.output);
    return r.code === 0 ? pass(r.output.split('\n').filter((l) => /^(OK|manifest)/.test(l)).join('; ')) : fail(r.output.slice(-500));
  });
}

// ----------------------------------------------- no WebView2 (R38)

async function noWebView2Phase() {
  await check('R38', 'On a machine without WebView2, the installer installs it first and LeGaJ opens', async () => {
    const r = await win.call('run', { file: 'powershell.exe', args: ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', join(cfg.repo, 'scripts', 'sweep', 'remove-webview2.ps1')] });
    writeFileSync(join(cfg.out, 'remove-webview2.txt'), r.output);
    if (r.code !== 0) return skip(`WebView2 could not be removed from this runner: ${r.output.slice(-300)}`);
    // Setup runs with the desktop's ordinary rights, as a user's double-click
    // would, so the WebView2 bootstrapper runs without elevation too.
    const log = join(cfg.out, 'install-nowebview2.log');
    const lnk = join(cfg.work, 'setup-silent.lnk');
    await win.call('shortcut', { path: lnk, target: cfg.setup, arguments: `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART "/LOG=${log}"` });
    await win.call('shell', { path: lnk });
    const text = await until('Setup to finish', () => (existsSync(log) && /Log closed\./.test(readFileSync(log, 'utf8')) ? readFileSync(log, 'utf8') : null), 600000, 2000);
    const problems = [];
    if (!/Installation process succeeded/.test(text)) problems.push('the log does not show success');
    if (!/MicrosoftEdgeWebview2Setup/i.test(text)) problems.push('the log shows no WebView2 step');
    const client = 'Microsoft\\EdgeUpdate\\Clients\\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
    let pv = null;
    for (const key of [`HKLM:\\SOFTWARE\\WOW6432Node\\${client}`, `HKLM:\\SOFTWARE\\${client}`, `HKCU:\\Software\\${client}`]) {
      const v = await win.call('regvalue', { key, name: 'pv' });
      if (v && v !== '0.0.0.0') pv = `${v} (${key.split('\\')[0]})`;
    }
    if (!pv) problems.push('WebView2 is still missing after the install');
    if (!problems.length) {
      await startApp();
      await waitText(/Set up LeGaJ/, 30000);
      await shot('after-webview2-install');
      await quitApp();
    }
    return problems.length ? fail(problems.join('; ')) : pass(`WebView2 ${pv} installed by Setup, without elevation; the wizard opened`);
  });
}

// ================================================================== run

try {
  if (cfg.phase === 'main') await mainPhase();
  else if (cfg.phase === 'standard') await standardPhase();
  else await noWebView2Phase();
} catch (err) {
  record('sweep', 'driver', 'FAIL', `stopped: ${err.stack ?? err.message}`);
  await shot('driver-stopped').catch(() => {});
} finally {
  try {
    saveAppLogs();
  } catch {
    // nothing to keep
  }
  writeFileSync(join(cfg.out, 'results.json'), JSON.stringify(results, null, 2));
  const mark = { PASS: 'PASS', FAIL: '**FAIL**', SKIP: 'SKIP', INFO: 'info' };
  const md = [`## Release sweep: ${cfg.phase}`, '', '| Check | Item | Result | Detail |', '|---|---|---|---|',
    ...results.map((r) => `| ${r.id} | ${r.item.replaceAll('|', '/')} | ${mark[r.status] ?? r.status} | ${r.detail.replaceAll('|', '/')} |`), ''];
  writeFileSync(join(cfg.out, 'summary.md'), md.join('\n'));
  win.close();
  pages.close();
  process.exitCode = results.some((r) => r.status === 'FAIL') ? 1 : 0;
  setTimeout(() => process.exit(), 2000).unref();
}
