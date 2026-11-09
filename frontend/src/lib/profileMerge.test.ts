import { describe, expect, it } from 'vitest';

import type { Profile } from './api';
import { mergeDraft, replacesHistory } from './profileMerge';

const empty = (): Profile => ({
  fullName: '', email: '', phone: '', location: '', headline: '', targetRole: '', summary: '',
  links: [], experiences: [], education: [], skills: [],
});

const saved = (): Profile => ({
  ...empty(),
  fullName: 'Ada Lovelace',
  email: 'ada@old.example',
  summary: 'Hand-written summary.',
  targetRole: 'Junior Data Engineer',
  links: [{ label: 'Site', url: 'https://ada.example' }],
  skills: [{ category: 'Math', items: ['Calculus'] }],
  experiences: [{ id: 'e1', company: 'Old Co', title: 'Clerk', location: '', start: { year: 1840, month: 0 }, end: { year: 0, month: 0 }, current: false, bullets: [] }],
});

const draft = (): Profile => ({
  ...empty(),
  fullName: 'Ada King',
  email: 'ada@new.example',
  links: [{ label: 'GitHub', url: 'https://github.com/ada' }, { label: 'Site', url: 'https://ada.example' }],
  experiences: [{ id: '', company: 'Babbage & Co', title: 'Analyst', location: '', start: { year: 1842, month: 1 }, end: { year: 1843, month: 9 }, current: false, bullets: ['Notes A-G'] }],
});

describe('R9/R7 merging a resume draft into a saved profile (gate CR1)', () => {
  it('keeps what the draft lacks and takes the history the draft has', () => {
    const m = mergeDraft(saved(), draft());
    expect(m.fullName).toBe('Ada King');
    expect(m.email).toBe('ada@new.example');
    expect(m.summary).toBe('Hand-written summary.');
    expect(m.targetRole).toBe('Junior Data Engineer'); // never blanked (U1)
    expect(m.links.map((l) => l.url)).toEqual(['https://ada.example', 'https://github.com/ada']);
    expect(m.skills).toEqual([{ category: 'Math', items: ['Calculus'] }]);
    expect(m.experiences.map((e) => e.company)).toEqual(['Babbage & Co']);
  });

  it('says when saving would replace history the user already has', () => {
    expect(replacesHistory(saved(), draft())).toBe(true);
    expect(replacesHistory(empty(), draft())).toBe(false);
    expect(replacesHistory(saved(), { ...draft(), experiences: [] })).toBe(false);
  });
});
