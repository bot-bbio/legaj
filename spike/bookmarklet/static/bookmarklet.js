(() => {
  const O = 'http://127.0.0.1:__PORT__';
  const M = '__METHOD__';
  const d = document;
  const t0 = Date.now();
  const ld = Array.from(d.querySelectorAll('script[type="application/ld+json"]')).slice(0, 20).map((s) => s.textContent || '');
  const meta = {};
  d.querySelectorAll('meta[property],meta[name]').forEach((m) => {
    const k = m.getAttribute('property') || m.getAttribute('name') || '';
    if (/^(og:|twitter:|description$|title$|author$)/i.test(k)) {
      meta[k] = (m.getAttribute('content') || '').slice(0, 2000);
    }
  });
  const root = d.querySelector('main') || d.body;
  const payload = {
    type: 'legaj-clip',
    v: 1,
    method: M,
    url: location.href,
    title: d.title,
    jsonld: ld,
    meta: meta,
    h1: Array.from(d.querySelectorAll('h1')).slice(0, 5).map((h) => (h.innerText || '').trim()),
    selection: String(window.getSelection() || '').slice(0, 20000),
    text: (root ? root.innerText : '').slice(0, 300000),
    collectMs: Date.now() - t0,
  };
  const feats = 'popup,width=560,height=680';
  if (M === 'frag') {
    const wf = window.open(O + '/clip?m=frag#' + encodeURIComponent(JSON.stringify(payload)), 'legaj_clip', feats);
    if (!wf) {
      alert('LeGaJ spike: the popup was blocked.');
    }
    return;
  }
  const w = window.open(O + '/clip?m=pm', 'legaj_clip', feats);
  if (!w) {
    alert('LeGaJ spike: the popup was blocked.');
    return;
  }
  const on = (e) => {
    if (e.source !== w || e.origin !== O) {
      return;
    }
    if (e.data && e.data.type === 'legaj-ready') {
      w.postMessage(payload, O);
      window.removeEventListener('message', on);
    }
  };
  window.addEventListener('message', on);
  setTimeout(() => window.removeEventListener('message', on), 20000);
})();
