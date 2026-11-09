import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { RoundView } from '../lib/api';

const base: RoundView = {
  id: 31,
  appId: 7,
  company: 'Acme',
  role: 'Dev',
  stage: 'interviewing',
  local: '2026-11-03T18:00',
  zone: 'America/New_York',
  startsAt: '2026-11-03T23:00:00Z',
  endsAt: '2026-11-04T00:00:00Z',
  durationMin: 60,
  kind: 'video',
  location: '',
  link: '',
  interviewers: [],
  prepNotes: '',
  postNotes: '',
  outcome: '',
  questions: [],
};

const api = {
  suggestQuestions: vi.fn<(id: number) => Promise<RoundView>>(),
  copyText: vi.fn<(t: string) => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: QuestionsPanel } = await import('./QuestionsPanel.svelte');

describe('R32 questions to ask', () => {
  beforeEach(() => vi.clearAllMocks());

  it('is disabled unless the application is in Interviewing', () => {
    render(QuestionsPanel, { round: { ...base, stage: 'offer' } });
    expect(screen.getByRole('button', { name: /suggest questions/i })).toBeDisabled();
    expect(screen.getByText(/only for applications in interviewing/i)).toBeInTheDocument();
  });

  it('generates on request, shows them, and copies them all', async () => {
    const user = userEvent.setup();
    const qs = Array.from({ length: 8 }, (_, i) => `Question ${i + 1}?`);
    api.suggestQuestions.mockResolvedValue({ ...base, questions: qs });
    render(QuestionsPanel, { round: base });
    expect(api.suggestQuestions).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: /suggest questions/i }));
    expect(api.suggestQuestions).toHaveBeenCalledWith(31);
    expect(await screen.findByText('Question 8?')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /copy all/i }));
    expect(api.copyText).toHaveBeenCalledWith(qs.map((q) => `- ${q}`).join('\n'));
  });

  it('shows saved questions and the backend reason on failure', async () => {
    const user = userEvent.setup();
    api.suggestQuestions.mockRejectedValue('Gemini is busy right now. Try again in a minute.');
    render(QuestionsPanel, { round: { ...base, questions: ['Saved question?'] } });
    expect(screen.getByText('Saved question?')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /suggest questions again/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/gemini is busy/i);
    expect(screen.getByText('Saved question?')).toBeInTheDocument();
  });
});
