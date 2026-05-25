(async function() {
  var h = window.location.hostname, p = window.location.href, c = '', r = '', l = '', d = '';
  try {
    if (h.includes('linkedin.com')) {
      var t = document.title.replace(' | LinkedIn', '').split(' at ');
      c = (document.querySelector('[data-tracking-control-name="public_jobs_topcard-org-name"]') || document.querySelector('.topcard__org-name-link') || document.querySelector('.job-details-jobs-unified-top-card__company-name a') || { innerText: t[1] || '' }).innerText;
      r = (document.querySelector('h1.t-24') || document.querySelector('.topcard__title') || document.querySelector('.job-details-jobs-unified-top-card__job-title h1') || { innerText: t[0] || '' }).innerText;
      l = (document.querySelector('.topcard__flavor--bullet') || document.querySelector('.job-details-jobs-unified-top-card__bullet') || { innerText: '' }).innerText;
      d = (document.querySelector('.description__text') || document.querySelector('#job-details') || { innerText: '' }).innerText;
    } else if (h.includes('indeed.com')) {
      var t2 = document.title.replace(' - Indeed', '').split(' at ');
      r = (document.querySelector('[data-testid="jobsearch-JobInfoHeader-title"]') || document.querySelector('h1.jobsearch-JobInfoHeader-title') || { innerText: t2[0] || '' }).innerText;
      c = (document.querySelector('[data-company-name]') || document.querySelector('[data-testid="inlineHeader-companyName"]') || { innerText: t2[1] || '' }).innerText;
      l = (document.querySelector('[data-testid="inlineHeader-companyLocation"]') || { innerText: '' }).innerText;
      d = (document.querySelector('#jobDescriptionText') || { innerText: '' }).innerText;
    } else if (h.includes('greenhouse.io') || h.includes('grnh.se')) {
      var t3 = document.title.split(' - ');
      r = (document.querySelector('h1.app-title') || document.querySelector('.app__title h1') || { innerText: t3[0] || '' }).innerText;
      c = (document.querySelector('.company-name') || { innerText: t3[t3.length-1] || '' }).innerText;
      l = (document.querySelector('.location') || { innerText: '' }).innerText;
      d = (document.querySelector('#content') || { innerText: '' }).innerText;
    } else if (h.includes('lever.co')) {
      var t4 = document.title.split('|');
      r = (document.querySelector('h2') || { innerText: t4[0] || '' }).innerText;
      c = t4.length > 1 ? t4[t4.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.sort-by-time.posting-category') || { innerText: '' }).innerText;
      d = (document.querySelector('.section-wrapper') || { innerText: '' }).innerText;
    } else if (h.includes('myworkdayjobs.com') || h.includes('workday.com')) {
      var t5 = document.title.split('|');
      r = (document.querySelector('[data-automation-id="jobPostingHeader"]') || { innerText: t5[0] || '' }).innerText;
      c = t5.length > 1 ? t5[t5.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('[data-automation-id="locations"]') || { innerText: '' }).innerText;
      d = (document.querySelector('[data-automation-id="jobPostingDescription"]') || { innerText: '' }).innerText;
    } else if (h.includes('ashbyhq.com')) {
      var t6 = document.title.split(' at ');
      r = (document.querySelector('h1') || { innerText: t6[0] || '' }).innerText;
      c = t6.length > 1 ? t6[t6.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.ashby-job-posting-brief-location') || { innerText: '' }).innerText;
      d = (document.querySelector('.ashby-job-posting-description') || { innerText: '' }).innerText;
    } else if (h.includes('icims.com')) {
      var t7 = document.title.split('-');
      r = (document.querySelector('h1') || { innerText: t7[0] || '' }).innerText;
      c = t7.length > 1 ? t7[t7.length-1].trim() : h.split('.')[0];
      l = (document.querySelector('.iCIMS_InfoMsg') || { innerText: '' }).innerText;
      d = (document.querySelector('#jobDetails') || { innerText: '' }).innerText;
    }

    if (!r) {
      var sel = ['.job-title', '.posting-title', '.title', '.role', 'h1'];
      for (var i = 0; i < sel.length; i++) {
        var el = document.querySelector(sel[i]);
        if (el && el.innerText.trim()) { r = el.innerText.trim(); break; }
      }
      if (!r) {
        var og = document.querySelector('meta[property="og:title"]');
        if (og && og.content.trim()) r = og.content.trim();
      }
      if (!r && document.title) {
        var ct = document.title.replace(/\s*[-|•|\|]\s*(LinkedIn|Indeed|ZipRecruiter|Glassdoor|Google).*$/i, '').trim();
        r = ct;
      }
      if (!r || r.toLowerCase().includes('apply now') || r.toLowerCase().includes('current openings') || r.toLowerCase().includes('career page') || r.toLowerCase().includes('job details')) {
        var segs = window.location.pathname.split('/').filter(Boolean);
        for (var j = 0; j < segs.length; j++) {
          var s = segs[j];
          if (s.length > 5 && !s.includes('.') && isNaN(s)) {
            r = decodeURIComponent(s).replace(/[-_]/g, ' ').replace(/\b\w/g, function(x) { return x.toUpperCase(); });
            break;
          }
        }
      }
    }

    if (!c) {
      var ht = document.title;
      c = ht.includes(' at ') ? ht.split(' at ').pop().split('|')[0].split('-')[0].trim() : '';
    }

    if (!l) {
      var els = document.querySelectorAll('*');
      for (var i = 0; i < els.length; i++) {
        var el = els[i];
        if (el.children.length === 0 && el.innerText.trim()) {
          var idc = (el.id + ' ' + el.className).toLowerCase();
          if (idc.includes('location') || idc.includes('workplace') || idc.includes('remote') || idc.includes('hybrid')) {
            var txt = el.innerText.trim();
            if (txt.length > 2 && txt.length < 100) { l = txt; break; }
          }
        }
      }
      if (!l) {
        var geoRegex = /^[A-Z][a-zA-Z\s.]+,\s*[A-Z]{2}(\s+[A-Z][a-zA-Z\s.]+)?$/;
        var tags = document.querySelectorAll('h2, h3, h4, p, span');
        for (var i = 0; i < tags.length; i++) {
          var txt = tags[i].innerText.trim();
          if (geoRegex.test(txt) || txt.toLowerCase() === 'remote' || txt.toLowerCase().includes('remote,') || txt.toLowerCase().includes('hybrid,')) {
            l = txt;
            break;
          }
        }
      }
    }

    if (!d) {
      var selD = ['main', 'article', '[class*="description"]', '[id*="description"]', '#job-details', '#content', '.section-wrapper', '[data-automation-id="jobPostingDescription"]', '.ashby-job-posting-description', '#jobDescriptionText'];
      for (var i = 0; i < selD.length; i++) {
        var el = document.querySelector(selD[i]);
        if (el && el.innerText.trim()) { d = el.innerText.trim(); break; }
      }
      if (!d) d = document.body.innerText;
    }
  } catch (e) {}

  c = (c || '').trim().substring(0, 100);
  r = (r || '').trim().substring(0, 150);
  l = (l || '').trim().substring(0, 100);
  d = (d || '').trim().substring(0, 50000);

  if (!c && !r) {
    alert("❌ Could not scrape job details from this page.");
    return;
  }

  try {
    let res = await fetch('http://127.0.0.1:8081/clip', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
        'X-LeGaJ-Token': '7b9ef3f04c4a6801533c82d9246c0871'
      },
      body: JSON.stringify({ company: c, role: r || document.title, location: l, link: p, description: d })
    });
    if (res.ok) {
      alert('✅ Clipped successfully via LeGaJ Extension!');
    } else {
      alert('❌ Failed to send clip. Server returned: ' + res.status);
    }
  } catch (err) {
    alert('❌ Failed to send clip. Is LeGaJ running?');
  }
})();