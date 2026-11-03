import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Answer, ApplicationRow, Suggestion } from '../lib/api';
import { NAV_SECTIONS } from '../lib/nav';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const row = (id: number, company: string): ApplicationRow => ({
  id,
  company,
  role: `Role ${id}`,
  location: '',
  url: '',
  stage: 'lead',
  source: 'clip',
  materials: 'answers',
  createdAt: '2026-11-01T09:00:00Z',
  appliedAt: '',
});

const draft: Answer = {
  id: 11,
  appId: 7,
  question: 'Why do you want to work at Acme?',
  answer: '',
  options: ['Option one about Acme.', 'Option two about Acme.', 'Option three about Acme.'],
  status: 'draft',
  inBank: false,
  updated: '2026-11-02T09:00:00Z',
};

const api = {
  listApplications: vi.fn<() => Promise<ApplicationRow[]>>(),
  listAnswers: vi.fn<(appId: number) => Promise<Answer[]>>(),
  generateAnswers: vi.fn<(appId: number, q: string) => Promise<Answer>>(),
  approveAnswer: vi.fn<(id: number, text: string) => Promise<Answer>>(),
  deleteAnswer: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  answerBank: vi.fn<() => Promise<Answer[]>>(),
  suggestAnswers: vi.fn<(q: string) => Promise<Suggestion[]>>(),
  removeFromBank: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  copyText: vi.fn<(text: string) => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: Pipeline } = await import('./Pipeline.svelte');

async function openAnswers(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  render(Pipeline);
  await user.click(await screen.findByRole('button', { name: /answers for acme/i }));
  return screen.findByRole('complementary', { name: /answers for acme/i });
}

describe('R25 answers inside Pipeline', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([row(7, 'Acme')]);
    api.listAnswers.mockResolvedValue([draft]);
    api.answerBank.mockResolvedValue([]);
    api.suggestAnswers.mockResolvedValue([]);
  });

  it('the copy button copies that option', async () => {
    const user = userEvent.setup();
    const panel = await openAnswers(user);
    await user.click(await within(panel).findByRole('button', { name: /copy option 2/i }));
    expect(api.copyText).toHaveBeenCalledWith('Option two about Acme.');
    expect(await within(panel).findByText(/copied/i)).toBeInTheDocument();
  });

  it('approve, then the bank shows the answer', async () => {
    const user = userEvent.setup();
    const approved: Answer = { ...draft, answer: 'Option two about Acme.', status: 'approved', inBank: true };
    api.approveAnswer.mockImplementation(() => {
      api.answerBank.mockResolvedValue([approved]);
      return Promise.resolve(approved);
    });
    const panel = await openAnswers(user);
    await user.click(await within(panel).findByRole('button', { name: /approve option 2/i }));
    expect(api.approveAnswer).toHaveBeenCalledWith(11, 'Option two about Acme.');

    await user.click(screen.getByRole('tab', { name: /answer bank/i }));
    const bank = await screen.findByRole('tabpanel', { name: /answer bank/i });
    expect(await within(bank).findByText('Option two about Acme.')).toBeInTheDocument();
    expect(within(bank).getByText('Why do you want to work at Acme?')).toBeInTheDocument();
  });

  it('an edited option is approved as edited', async () => {
    const user = userEvent.setup();
    api.approveAnswer.mockResolvedValue({ ...draft, status: 'approved' });
    const panel = await openAnswers(user);
    const box = await within(panel).findByRole('textbox', { name: /option 1/i });
    await user.clear(box);
    await user.type(box, 'My own words about Acme.');
    await user.click(within(panel).getByRole('button', { name: /approve option 1/i }));
    expect(api.approveAnswer).toHaveBeenCalledWith(11, 'My own words about Acme.');
  });

  it('the suggestion appears for a similar question, before generating', async () => {
    const user = userEvent.setup();
    const banked: Answer = {
      ...draft,
      id: 3,
      appId: 0,
      question: 'Why do you want to join Globex?',
      answer: 'A banked answer from before.',
      status: 'approved',
      inBank: true,
    };
    api.suggestAnswers.mockResolvedValue([{ answer: banked, score: 0.6 }]);
    const panel = await openAnswers(user);
    await user.type(within(panel).getByRole('textbox', { name: /^question/i }), 'Why do you want to join Acme?');

    const found = await within(panel).findByRole('region', { name: /from your answer bank/i });
    expect(api.suggestAnswers).toHaveBeenLastCalledWith('Why do you want to join Acme?');
    expect(within(found).getByText('A banked answer from before.')).toBeInTheDocument();
    await user.click(within(found).getByRole('button', { name: /copy/i }));
    expect(api.copyText).toHaveBeenCalledWith('A banked answer from before.');
    expect(api.generateAnswers).not.toHaveBeenCalled();
  });

  it('Generate sends the question; empty means why-this-company', async () => {
    const user = userEvent.setup();
    api.generateAnswers.mockResolvedValue(draft);
    const panel = await openAnswers(user);
    await user.click(within(panel).getByRole('button', { name: /generate answers/i }));
    expect(api.generateAnswers).toHaveBeenCalledWith(7, '');
  });

  it('R1 the nav still has exactly 5 sections', () => {
    expect(NAV_SECTIONS.map((s) => s.label)).toEqual(['Profile', 'Pipeline', 'Follow-ups', 'Interviews', 'Settings']);
  });
});
