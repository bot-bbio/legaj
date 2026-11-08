// A stand-in for the Gemini API, for the release sweep only. The runner's
// hosts file points generativelanguage.googleapis.com here, and the sweep
// trusts this server's certificate, so the installed app talks to it exactly
// as it would to Google. Replies are fixed, schema-valid text built from
// made-up sentinel values; no real key or personal data is involved.
//
// Usage: node fake-gemini.mjs <config.json>
// config: { pfx, passphrase, key, log, profile: { name, email, phone, line } }
import { appendFileSync, readFileSync } from 'node:fs';
import { createServer } from 'node:https';
import { readJSON } from './cdp.mjs';

const cfg = readJSON(process.argv[2]);
const p = cfg.profile;

const note = (entry) => appendFileSync(cfg.log, JSON.stringify({ at: new Date().toISOString(), ...entry }) + '\n');

// Words without names, so no validator sees another company in them.
const filler = 'I enjoy turning a messy question into a plan that a team can act on, and I write down what I learn so the next person starts ahead. In my last role I took on work nobody owned, made it measurable, and handed it over in better shape than I found it. I am at my best when the goal is clear, the feedback is honest, and the people around me care about doing the job well. I would bring the same steady habits here: listen first, ship small improvements often, and keep everyone informed about what changed and why it matters to them.';

function companyOf(prompt) {
  const m = /role at "((?:[^"\\]|\\.)*)"/.exec(prompt);
  if (!m) return '';
  try {
    return JSON.parse(`"${m[1]}"`);
  } catch {
    return m[1];
  }
}

function reply(schema, prompt) {
  const props = schema?.properties ?? {};
  if (!schema) return 'OK';
  if ('fullName' in props) {
    return {
      fullName: p.name, email: p.email, phone: p.phone, location: 'Gdansk, Poland',
      headline: 'Operations analyst', summary: `${p.line}. Analyst who turns data into decisions.`,
      links: [],
      experiences: [{
        company: 'Sentinel Works', title: 'Operations Analyst', location: 'Gdansk, Poland',
        start: '2021-03', end: '', current: true,
        bullets: [p.line, 'Cut the weekly reporting time from two days to two hours.'],
      }],
      education: [{ school: 'Sentinel University', degree: 'BSc', field: 'Economics', location: 'Gdansk', start: '2015', end: '2019', details: '' }],
      skills: [{ category: 'Tools', items: ['SQL', 'Excel', 'Python'] }],
    };
  }
  if ('whyOptions' in props) {
    const company = companyOf(prompt) || 'your team';
    return {
      salutation: 'Dear Hiring Team,',
      opening: `I am writing to apply for this role at ${company}, because the work it describes is the work I do best.`,
      whyOptions: [
        `${company} builds things people rely on, and I want my analysis to matter to that many people.`,
        `What I have read about how ${company} works, in small teams with clear goals, matches how I work best.`,
        `The problems ${company} is solving are the ones I have been preparing for in every role so far.`,
      ],
      body: [filler],
      closing: 'Thank you for your time. I would welcome the chance to talk about how I could help.',
      signOff: 'Kind regards,',
    };
  }
  if ('paragraph' in props) return { paragraph: `${filler} I keep my promises small and keep them often.` };
  if ('answers' in props) return { answers: [filler, `${filler} That is the habit I want to keep.`, `${filler} It is also why this role appeals to me.`] };
  if ('questions' in props) {
    return {
      questions: [
        'What does success look like in the first ninety days?', 'How does the team decide what to work on next?',
        'What is the hardest problem the team is facing this year?', 'How is feedback shared on this team?',
        'Which tools does the team use every day?', 'How do analysts and engineers work together here?',
        'What would you change about the role if you could?', 'How has the team grown over the last year?',
        'What do people who do well here have in common?', 'What are the next steps after this interview?',
      ],
    };
  }
  if ('company' in props && 'role' in props) return { company: '', role: '', location: '' };
  return null;
}

const server = createServer({ pfx: readFileSync(cfg.pfx), passphrase: cfg.passphrase }, (req, res) => {
  const chunks = [];
  req.on('data', (c) => chunks.push(c));
  req.on('end', () => {
    const body = Buffer.concat(chunks).toString('utf8');
    const url = req.url ?? '';
    const key = req.headers['x-goog-api-key'];
    const entry = {
      method: req.method, path: url.replace(/\?.*$/, ''),
      keyInHeader: key === cfg.key, keyInUrl: url.includes(cfg.key), keyInBody: body.includes(cfg.key),
      bytes: body.length,
    };
    const send = (status, obj) => {
      res.writeHead(status, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(obj));
    };
    if (req.method !== 'POST' || !/^\/v1beta\/models\/[\w.-]+:generateContent$/.test(entry.path)) {
      note({ ...entry, status: 404 });
      return send(404, { error: { code: 404, message: 'Not found', status: 'NOT_FOUND' } });
    }
    if (key !== cfg.key) {
      note({ ...entry, status: 400 });
      return send(400, { error: { code: 400, message: 'API key not valid. Please pass a valid API key.', status: 'INVALID_ARGUMENT', details: [{ reason: 'API_KEY_INVALID' }] } });
    }
    let parsed;
    try {
      parsed = JSON.parse(body);
    } catch {
      note({ ...entry, status: 400, why: 'bad json' });
      return send(400, { error: { code: 400, message: 'Invalid JSON payload', status: 'INVALID_ARGUMENT' } });
    }
    const schema = parsed.generationConfig?.responseJsonSchema;
    const prompt = (parsed.contents ?? []).flatMap((c) => c.parts ?? []).map((x) => x.text ?? '').join('\n');
    const out = reply(schema, prompt);
    if (out === null) {
      note({ ...entry, status: 500, why: 'unknown task' });
      return send(500, { error: { code: 500, message: 'Internal error', status: 'INTERNAL' } });
    }
    const task = !schema ? 'key-test' : Object.keys(schema.properties ?? {}).slice(0, 2).join(',');
    note({ ...entry, status: 200, task });
    // Letters take a while, as Gemini's do, so the focus check (R6) has
    // time to watch the generation.
    const delay = 'whyOptions' in (schema?.properties ?? {}) ? 1500 : 0;
    setTimeout(() => send(200, {
      candidates: [{ content: { role: 'model', parts: [{ text: typeof out === 'string' ? out : JSON.stringify(out) }] }, finishReason: 'STOP' }],
      modelVersion: 'sweep-fake',
    }), delay);
  });
});
server.listen(443, '127.0.0.1', () => console.log('fake gemini listening on 127.0.0.1:443'));
