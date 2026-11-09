import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { DueView, FollowupDraft, FollowupInput, FollowupKind, MailClient } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const nudge: DueView = {
  kind: 'applied_nudge',
  appId: 1,
  roundId: 0,
  company: 'Acme',
  role: 'Dev',
  interviewer: '',
  dueSince: '2026-11-02T00:00:00Z',
  daysSince: 8,
};
const thanks: DueView = { ...nudge, kind: 'thank_you', appId: 2, roundId: 5, company: 'Globex', daysSince: 0 };
const flagged: DueView = { ...nudge, kind: 'thank_you', appId: 3, roundId: 6, company: 'Initech' };

const draftFor = (d: DueView, problems: string[] = []): FollowupDraft => ({
  kind: d.kind,
  appId: d.appId,
  roundId: d.roundId,
  to: '',
  subject: `About ${d.company}`,
  body: `Hello ${d.company}`,
  problems,
  warnings: [],
});

const api = {
  followupsDue: vi.fn<() => Promise<DueView[]>>(),
  followupDraft: vi.fn<(k: FollowupKind, appId: number, roundId: number) => Promise<FollowupDraft>>(),
  saveFollowupDraft: vi.fn<(d: FollowupInput) => Promise<FollowupDraft>>(),
  openCompose: vi.fn<(d: FollowupInput, c: MailClient) => Promise<void>>(() => Promise.resolve()),
  logFollowup: vi.fn<(d: FollowupInput, s: 'sent' | 'skipped') => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: FollowUps } = await import('./FollowUps.svelte');

describe('R29 Follow-ups view', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.followupsDue.mockResolvedValue([nudge, thanks, flagged]);
    api.followupDraft.mockImplementation((_k, appId) => {
      const d = [nudge, thanks, flagged].find((x) => x.appId === appId) ?? nudge;
      return Promise.resolve(draftFor(d, d === flagged ? ['Fill in {interviewer} before sending.'] : []));
    });
    api.saveFollowupDraft.mockImplementation((d) => Promise.resolve({ ...d, problems: [], warnings: [] }));
  });

  it('Open all due confirms, then the Sent? prompt logs an event', async () => {
    const user = userEvent.setup();
    render(FollowUps);
    await user.click(await screen.findByRole('button', { name: /open all due \(3\)/i }));

    // Nothing opens until the user confirms.
    expect(api.openCompose).not.toHaveBeenCalled();
    const confirm = screen.getByRole('alertdialog', { name: /open all due/i });
    await user.click(within(confirm).getByRole('button', { name: /^open all$/i }));

    expect(api.openCompose).toHaveBeenCalledTimes(2);
    expect(api.openCompose.mock.calls.map((c) => c[0].appId).sort()).toEqual([1, 2]);
    expect(api.openCompose.mock.calls.every((c) => c[1] === 'gmail')).toBe(true);
    // The flagged draft isn't opened; the user is told why.
    expect(await screen.findByText(/initech.*fill in \{interviewer\}/i)).toBeInTheDocument();

    const acme = screen.getByRole('article', { name: /acme/i });
    const prompt = within(acme).getByRole('group', { name: /sent\?/i });
    await user.click(within(prompt).getByRole('button', { name: /^sent$/i }));
    expect(api.logFollowup).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'applied_nudge', appId: 1, subject: 'About Acme', body: 'Hello Acme' }),
      'sent',
    );
  });

  it('Cancel opens nothing', async () => {
    const user = userEvent.setup();
    render(FollowUps);
    await user.click(await screen.findByRole('button', { name: /open all due/i }));
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: /cancel/i }));
    expect(api.openCompose).not.toHaveBeenCalled();
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('one draft: edit, open in the chosen client, then skip logs skipped', async () => {
    const user = userEvent.setup();
    render(FollowUps);
    await user.selectOptions(await screen.findByRole('combobox', { name: /send with/i }), 'mailto');
    const globex = await screen.findByRole('article', { name: /globex/i });
    await user.click(within(globex).getByRole('button', { name: /write/i }));
    const to = await within(globex).findByRole('textbox', { name: /^to/i });
    await user.type(to, 'grace@example.com');
    await user.click(within(globex).getByRole('button', { name: /open in your mail app/i }));
    expect(api.openCompose).toHaveBeenCalledWith(expect.objectContaining({ appId: 2, to: 'grace@example.com' }), 'mailto');

    const prompt = within(globex).getByRole('group', { name: /sent\?/i });
    await user.click(within(prompt).getByRole('button', { name: /skip/i }));
    expect(api.logFollowup).toHaveBeenCalledWith(expect.objectContaining({ appId: 2, roundId: 5 }), 'skipped');
  });

  it('shows when nothing is due', async () => {
    api.followupsDue.mockResolvedValue([]);
    render(FollowUps);
    expect(await screen.findByText(/nothing is due/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /open all due/i })).not.toBeInTheDocument();
  });
});
