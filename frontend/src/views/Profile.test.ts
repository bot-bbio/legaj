import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Profile } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const stored = (): Profile => ({
  fullName: 'Ada Lovelace',
  email: 'ada@example.com',
  phone: '',
  location: 'London',
  headline: '',
  targetRole: 'Programmer',
  summary: '',
  links: [],
  experiences: [
    { id: 'e1', company: 'Babbage & Co', title: 'Analyst', location: 'London', start: { year: 1842, month: 1 }, end: { year: 1843, month: 9 }, current: false, bullets: ['Notes A-G'] },
  ],
  education: [
    { id: 'd1', school: 'Home', degree: 'Mathematics', field: '', location: 'London', start: { year: 0, month: 0 }, end: { year: 0, month: 0 }, details: 'Tutored by De Morgan.' },
  ],
  skills: [{ category: 'Math', items: ['Calculus'] }],
});

const api = {
  getProfile: vi.fn<() => Promise<Profile>>(),
  saveProfile: vi.fn<(p: Profile) => Promise<Profile>>(),
};
vi.mock('../lib/api', () => ({ api }));

const { default: ProfileView } = await import('./Profile.svelte');

function lastSaved(): Profile {
  const call = api.saveProfile.mock.calls.at(-1);
  if (!call) throw new Error('saveProfile was not called');
  return call[0];
}

describe('R9 profile view', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getProfile.mockResolvedValue(stored());
    api.saveProfile.mockImplementation((p) => Promise.resolve(p));
  });

  it('add and remove experiences, then save sends the full current list', async () => {
    render(ProfileView);
    await screen.findByDisplayValue('Babbage & Co');

    await userEvent.click(screen.getByRole('button', { name: /add experience/i }));
    const second = screen.getAllByRole('group', { name: /^experience \d/i })[1];
    if (!second) throw new Error('new experience not shown');
    await userEvent.type(within(second).getByLabelText(/company/i), 'Royal Society');
    await userEvent.type(within(second).getByLabelText(/title/i), 'Correspondent');

    const first = screen.getAllByRole('group', { name: /^experience \d/i })[0];
    if (!first) throw new Error('first experience missing');
    await userEvent.click(within(first).getByRole('button', { name: /remove/i }));
    await userEvent.click(screen.getByRole('button', { name: /save profile/i }));

    const saved = lastSaved();
    expect(saved.experiences).toHaveLength(1);
    expect(saved.experiences[0]).toMatchObject({ company: 'Royal Society', title: 'Correspondent' });
    // The rest of the profile goes along untouched.
    expect(saved.education[0]).toMatchObject({ location: 'London', details: 'Tutored by De Morgan.' });
  });

  it('skill categories and items are editable', async () => {
    render(ProfileView);
    const category = await screen.findByDisplayValue('Math');
    await userEvent.clear(category);
    await userEvent.type(category, 'Mathematics');
    const items = screen.getByDisplayValue('Calculus');
    await userEvent.clear(items);
    await userEvent.type(items, 'Calculus, Algebra');
    await userEvent.click(screen.getByRole('button', { name: /save profile/i }));

    expect(lastSaved().skills).toEqual([{ category: 'Mathematics', items: ['Calculus', 'Algebra'] }]);
  });

  it('dates are entered as YYYY-MM and saved structured (A5)', async () => {
    render(ProfileView);
    const exp = (await screen.findAllByRole('group', { name: /^experience \d/i }))[0];
    if (!exp) throw new Error('no experience');
    const start = within(exp).getByLabelText(/start/i);
    expect(start).toHaveValue('1842-01');
    await userEvent.clear(start);
    await userEvent.type(start, '1841-12');
    await userEvent.click(screen.getByRole('button', { name: /save profile/i }));

    expect(lastSaved().experiences[0]?.start).toEqual({ year: 1841, month: 12 });
    expect(lastSaved().experiences[0]?.end).toEqual({ year: 1843, month: 9 });
  });

  it('an invalid date blocks saving with a message', async () => {
    render(ProfileView);
    const exp = (await screen.findAllByRole('group', { name: /^experience \d/i }))[0];
    if (!exp) throw new Error('no experience');
    const end = within(exp).getByLabelText(/end/i);
    await userEvent.clear(end);
    await userEvent.type(end, '1843/09');

    expect(within(exp).getByText(/YYYY-MM/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save profile/i })).toBeDisabled();
  });

  it('bullets are edited one per line', async () => {
    render(ProfileView);
    const bullets = await screen.findByDisplayValue('Notes A-G');
    await userEvent.type(bullets, '{Enter}Built the first program');
    await userEvent.click(screen.getByRole('button', { name: /save profile/i }));
    expect(lastSaved().experiences[0]?.bullets).toEqual(['Notes A-G', 'Built the first program']);
  });
});
