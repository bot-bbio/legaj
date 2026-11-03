import { fireEvent, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApplicationRow, RoundInput, RoundLists, RoundView } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const acme: ApplicationRow = {
  id: 7,
  company: 'Acme',
  role: 'Dev',
  location: '',
  url: '',
  stage: 'applied',
  source: 'clip',
  materials: 'letter',
  createdAt: '2026-11-01T09:00:00Z',
  appliedAt: '2026-11-01T09:00:00Z',
};

const view = (r: RoundInput, id = 31): RoundView => ({
  ...r,
  id,
  company: 'Acme',
  role: 'Dev',
  startsAt: '2026-11-03T23:00:00Z',
  endsAt: '2026-11-04T00:00:00Z',
  durationMin: r.durationMin || 60,
  questions: [],
});

const api = {
  listApplications: vi.fn<() => Promise<ApplicationRow[]>>(),
  interviewLists: vi.fn<() => Promise<RoundLists>>(),
  addRound: vi.fn<(r: RoundInput) => Promise<RoundView>>(),
  updateRound: vi.fn<(id: number, r: RoundInput) => Promise<RoundView>>(),
  deleteRound: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  addToCalendar: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  openURL: vi.fn<(u: string) => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: Interviews } = await import('./Interviews.svelte');

describe('R30 Interviews view', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([acme]);
    api.interviewLists.mockResolvedValue({ upcoming: [], past: [] });
    api.addRound.mockImplementation((r) => {
      const v = view(r);
      api.interviewLists.mockResolvedValue({ upcoming: [v], past: [] });
      return Promise.resolve(v);
    });
  });

  it('create a round and it appears under Upcoming; Add to calendar calls export', async () => {
    const user = userEvent.setup();
    render(Interviews);
    await user.click(await screen.findByRole('button', { name: /add round/i }));
    const form = screen.getByRole('form', { name: /interview round/i });

    await user.selectOptions(await within(form).findByRole('combobox', { name: /application/i }), '7');
    await fireEvent.input(within(form).getByLabelText(/date and time/i), { target: { value: '2026-11-03T18:00' } });
    await user.selectOptions(within(form).getByRole('combobox', { name: /time zone/i }), 'America/New_York');
    await user.selectOptions(within(form).getByRole('combobox', { name: /type/i }), 'video');
    await user.click(within(form).getByRole('button', { name: /add interviewer/i }));
    await user.type(within(form).getByRole('textbox', { name: /interviewer 1 name/i }), 'Grace Hopper');
    await user.click(within(form).getByRole('button', { name: /save round/i }));

    expect(api.addRound).toHaveBeenCalledWith(
      expect.objectContaining({
        appId: 7,
        local: '2026-11-03T18:00',
        zone: 'America/New_York',
        kind: 'video',
        interviewers: [{ name: 'Grace Hopper', title: '', email: '' }],
      }),
    );
    const upcoming = await screen.findByRole('region', { name: /upcoming/i });
    const card = await within(upcoming).findByRole('article', { name: /acme/i });
    expect(card).toHaveTextContent('Grace Hopper');
    expect(card).toHaveTextContent(/america\/new_york/i);

    await user.click(within(card).getByRole('button', { name: /add to calendar/i }));
    expect(api.addToCalendar).toHaveBeenCalledWith(31);
  });

  it('shows the backend reason when a round is refused', async () => {
    const user = userEvent.setup();
    api.addRound.mockRejectedValue('Enter the interview’s date and time.');
    render(Interviews);
    await user.click(await screen.findByRole('button', { name: /add round/i }));
    const form = screen.getByRole('form', { name: /interview round/i });
    await user.selectOptions(await within(form).findByRole('combobox', { name: /application/i }), '7');
    await user.click(within(form).getByRole('button', { name: /save round/i }));
    expect(await within(form).findByRole('alert')).toHaveTextContent(/date and time/i);
  });

  it('lists past rounds separately', async () => {
    const past = view(
      {
        appId: 7,
        local: '2026-10-20T09:00',
        zone: 'UTC',
        durationMin: 60,
        kind: 'phone',
        location: '',
        link: '',
        interviewers: [],
        prepNotes: '',
        postNotes: 'Went well.',
        outcome: 'Passed',
      },
      12,
    );
    api.interviewLists.mockResolvedValue({ upcoming: [], past: [past] });
    render(Interviews);
    const region = await screen.findByRole('region', { name: /past/i });
    const card = await within(region).findByRole('article', { name: /acme/i });
    expect(card).toHaveTextContent('Passed');
    expect(screen.getByRole('region', { name: /upcoming/i })).toHaveTextContent(/no upcoming interviews/i);
  });
});
