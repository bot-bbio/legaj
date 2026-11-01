'use strict';

(() => {
  const method = new URLSearchParams(location.search).get('m') === 'frag' ? 'frag' : 'pm';
  const t0 = performance.now();
  const $ = (id) => document.getElementById(id);
  let payload = null;

  function setStatus(text, kind) {
    const el = $('status');
    el.textContent = text;
    el.className = 'status ' + (kind || '');
  }

  function report(status, extra) {
    const body = Object.assign({ method: method, status: status, ms: Math.round(performance.now() - t0), ua: navigator.userAgent }, extra || {});
    fetch('/api/result', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }).catch(() => {});
  }

  function hostOf(u) {
    try {
      return new URL(u).host;
    } catch {
      return '';
    }
  }

  // The payload comes from an arbitrary web page: check every field's type
  // before using it.
  function valid(p) {
    return (
      p !== null &&
      typeof p === 'object' &&
      p.type === 'legaj-clip' &&
      p.v === 1 &&
      typeof p.url === 'string' &&
      typeof p.title === 'string' &&
      Array.isArray(p.jsonld) &&
      p.jsonld.every((s) => typeof s === 'string') &&
      Array.isArray(p.h1) &&
      typeof p.text === 'string' &&
      typeof p.selection === 'string'
    );
  }

  function accept(p, channel, extra) {
    if (!valid(p)) {
      report('invalid-payload', Object.assign({ channel: channel }, extra));
      setStatus('The page sent something that is not a valid clip.', 'bad');
      return;
    }
    payload = p;
    const summary = {
      channel: channel,
      host: hostOf(p.url),
      sizeBytes: new Blob([JSON.stringify(p)]).size,
      jsonldCount: p.jsonld.length,
      jobPosting: p.jsonld.some((s) => s.indexOf('JobPosting') !== -1),
      textChars: p.text.length,
      hasSelection: p.selection.length > 0,
      collectMs: typeof p.collectMs === 'number' ? p.collectMs : 0,
    };
    report('ok', Object.assign(summary, extra));
    $('s-host').textContent = summary.host;
    $('s-h1').textContent = String(p.h1[0] || p.title).slice(0, 200);
    $('s-channel').textContent = channel;
    $('s-ld').textContent = String(summary.jsonldCount);
    $('s-jp').textContent = summary.jobPosting ? 'yes' : 'no';
    $('s-text').textContent = summary.textChars.toLocaleString() + ' characters';
    $('s-size').textContent = (summary.sizeBytes / 1024).toFixed(1) + ' KB';
    $('summary').hidden = false;
    $('save').disabled = false;
    setStatus('Handoff worked.', 'good');
  }

  $('save').addEventListener('click', async () => {
    $('save').disabled = true;
    try {
      const r = await fetch('/api/save', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      $('saved').textContent = r.ok ? 'Saved. You can close this window.' : 'Save failed: HTTP ' + r.status;
    } catch (e) {
      $('saved').textContent = 'Save failed: ' + e;
    }
  });

  if (method === 'frag') {
    const raw = location.hash.slice(1);
    // Drop the payload from the address bar and history.
    history.replaceState(null, '', location.pathname + location.search);
    if (!raw) {
      report('empty-fragment');
      setStatus('No data arrived in the URL.', 'bad');
      return;
    }
    try {
      accept(JSON.parse(decodeURIComponent(raw)), 'fragment', { fragChars: raw.length });
    } catch {
      report('parse-error', { channel: 'fragment', fragChars: raw.length });
      setStatus('The URL data could not be read (likely truncated).', 'bad');
    }
    return;
  }

  if (!window.opener) {
    report('no-opener');
    setStatus('The job page cut the link to this window, probably via Cross-Origin-Opener-Policy. postMessage cannot work here.', 'bad');
    return;
  }

  const timer = setTimeout(() => {
    report('timeout');
    setStatus('The job page never answered (8 s). Check its console for errors.', 'bad');
  }, 8000);

  window.addEventListener('message', (e) => {
    if (e.source !== window.opener) {
      return;
    }
    if (!e.data || e.data.type !== 'legaj-clip') {
      return;
    }
    clearTimeout(timer);
    accept(e.data, 'postMessage', { openerOrigin: e.origin });
  });

  // "ready" carries no data, so '*' is safe: the opener's origin isn't known yet.
  window.opener.postMessage({ type: 'legaj-ready', v: 1 }, '*');
})();
