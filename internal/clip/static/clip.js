'use strict';

// The LeGaJ clip popup (R18). The bookmarklet opened this window from a job
// page, and that page sends its snapshot here with postMessage (D15, D33).
// The user checks it and clicks Save, and the snapshot goes to LeGaJ on this
// computer. Everything the page sends is untrusted: it's checked, and only
// ever shown as text.
(() => {
  const $ = (id) => document.getElementById(id);
  const session = document.querySelector('meta[name="legaj-session"]').content;
  let page = null;

  function setStatus(text, kind) {
    const el = $('status');
    el.textContent = text;
    el.className = 'status' + (kind ? ' ' + kind : '');
  }

  function hostOf(url) {
    try {
      return new URL(url).host;
    } catch {
      return '';
    }
  }

  function stringList(v, max) {
    return Array.isArray(v) && v.length <= max && v.every((s) => typeof s === 'string');
  }

  function valid(p) {
    return (
      p !== null &&
      typeof p === 'object' &&
      p.type === 'legaj-clip' &&
      p.v === 1 &&
      typeof p.url === 'string' &&
      p.url !== '' &&
      typeof p.title === 'string' &&
      stringList(p.jsonld, 20) &&
      stringList(p.h1, 5) &&
      typeof p.selection === 'string' &&
      typeof p.text === 'string' &&
      p.meta !== null &&
      typeof p.meta === 'object' &&
      !Array.isArray(p.meta) &&
      Object.values(p.meta).every((v) => typeof v === 'string')
    );
  }

  function accept(p) {
    // Keep only the fields LeGaJ reads.
    page = { type: 'legaj-clip', v: 1, url: p.url, title: p.title, jsonld: p.jsonld, meta: p.meta, h1: p.h1, selection: p.selection, text: p.text };
    $('page-title').textContent = String(p.h1[0] || p.title || 'Untitled page').slice(0, 200);
    $('page-host').textContent = hostOf(p.url);
    $('preview').hidden = false;
    $('save').hidden = false;
    $('save').focus();
    setStatus('Save this job to LeGaJ?');
  }

  function describe(r) {
    const job = [r.role, r.company].filter((s) => typeof s === 'string' && s !== '').join(' at ');
    if (!r.created) {
      return job ? 'Already in your pipeline: ' + job + '.' : 'This job is already in your pipeline.';
    }
    if (r.needsCompany) {
      const saved = job ? 'Saved ' + job + '. ' : 'Saved. ';
      return saved + (r.enriching ? 'LeGaJ is looking up the company name.' : 'Add the company name in LeGaJ.');
    }
    return 'Saved to LeGaJ: ' + job + '.';
  }

  $('save').addEventListener('click', async () => {
    $('save').disabled = true;
    setStatus('Saving…');
    try {
      const res = await fetch('/api/clips', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session: session, page: page }),
      });
      const body = await res.json().catch(() => ({}));
      if (res.ok && body.ok && body.result) {
        setStatus(describe(body.result), 'good');
        $('save').hidden = true;
        return;
      }
      setStatus(typeof body.message === 'string' && body.message !== '' ? body.message : "LeGaJ couldn't save this clip.", 'bad');
    } catch {
      setStatus("LeGaJ isn't answering. Make sure it's running, then click the bookmarklet again.", 'bad');
    }
  });

  if (!window.opener) {
    // The job page cut the link to this window (Cross-Origin-Opener-Policy, C43).
    setStatus("This page doesn't let LeGaJ read it. Open the job's own page, or use Paste job in LeGaJ.", 'bad');
    return;
  }

  const timer = setTimeout(() => {
    if (page === null) {
      setStatus("The job page didn't answer. Reload it, then click the bookmarklet again.", 'bad');
    }
  }, 8000);

  window.addEventListener('message', (e) => {
    // Only the page that opened this window, and only its first snapshot.
    if (e.source !== window.opener || page !== null || !e.data || e.data.type !== 'legaj-clip') {
      return;
    }
    clearTimeout(timer);
    if (!valid(e.data)) {
      setStatus("The page sent something LeGaJ can't read.", 'bad');
      return;
    }
    accept(e.data);
  });

  // "ready" carries no data, so any origin may hear it: the opener's origin
  // isn't known yet. The bookmarklet answers only a message from this origin.
  window.opener.postMessage({ type: 'legaj-ready', v: 1 }, '*');
})();
