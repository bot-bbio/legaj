'use strict';

(() => {
  const browser = (ua) => (ua.indexOf('Edg/') !== -1 ? 'Edge' : ua.indexOf('Chrome/') !== -1 ? 'Chrome' : 'other');

  async function load() {
    const res = await fetch('/api/results');
    const rows = (await res.json()) || [];
    const body = document.getElementById('rows');
    body.replaceChildren();
    rows
      .slice()
      .reverse()
      .forEach((r) => {
        const tr = document.createElement('tr');
        tr.className = r.status === 'ok' ? 'good' : 'bad';
        const cells = [
          new Date(r.time).toLocaleTimeString(),
          browser(r.ua || ''),
          r.host || '—',
          r.method + (r.channel ? ' (' + r.channel + ')' : ''),
          r.status,
          String(r.jsonldCount),
          r.jobPosting ? 'yes' : 'no',
          r.sizeBytes ? (r.sizeBytes / 1024).toFixed(1) + ' KB' : r.fragChars ? r.fragChars + ' chars' : '—',
          String(r.ms),
        ];
        cells.forEach((c) => {
          const td = document.createElement('td');
          td.textContent = c;
          tr.appendChild(td);
        });
        body.appendChild(tr);
      });
  }

  document.getElementById('refresh').addEventListener('click', load);
  load();
  setInterval(load, 5000);
})();
