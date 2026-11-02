import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type {
  ApplicationRow,
  ApproveAllResult,
  DocumentVersion,
  GenerateResult,
  Materials,
  ReviewItem,
} from '../lib/api';
import type { EntityChange } from '../lib/events';

// A controllable change feed, so a test can play the backend finishing a job.
const listeners = new Set<(c: EntityChange) => void>();
vi.mock('../lib/events', () => ({
  onEntityChanged: (cb: (c: EntityChange) => void) => {
    listeners.add(cb);
    return () => listeners.delete(cb);
  },
}));
function emit(c: EntityChange): void {
  for (const l of listeners) l(c);
}

const row = (id: number, company: string, stage: string, materials: Materials = 'letter'): ApplicationRow => ({
  id,
  company,
  role: `Role ${id}`,
  location: '',
  url: '',
  stage,
  source: 'clip',
  materials,
  createdAt: '2026-11-01T09:00:00Z',
  appliedAt: '',
});

const item = (appId: number, company: string, status: ReviewItem['status'], reason = ''): ReviewItem => ({
  appId,
  company,
  role: `Role ${appId}`,
  status,
  reason,
});

const api = {
  listApplications: vi.fn<() => Promise<ApplicationRow[]>>(),
  generateLetters: vi.fn<(ids: number[]) => Promise<GenerateResult>>(),
  reviewQueue: vi.fn<() => Promise<ReviewItem[]>>(),
  approveLetter: vi.fn<(id: number) => Promise<DocumentVersion>>(),
  approveAll: vi.fn<(ids: number[]) => Promise<ApproveAllResult>>(),
  dismissJob: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  setMaterials: vi.fn<(id: number, m: Materials) => Promise<ApplicationRow>>(),
};
vi.mock('../lib/api', () => ({ api }));

const { default: Pipeline } = await import('./Pipeline.svelte');
const { route } = await import('../lib/stores/route');

describe('R23/R43 generate and review inside Pipeline', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listeners.clear();
    api.listApplications.mockResolvedValue([row(7, 'Acme', 'lead'), row(8, 'Globex', 'lead', 'answers')]);
    api.reviewQueue.mockResolvedValue([]);
    api.generateLetters.mockImplementation((ids) => Promise.resolve({ queued: ids }));
    api.approveLetter.mockImplementation((id) =>
      Promise.resolve({ documentId: 1, appId: id, version: 1, path: `documents/${id}_Acme/Acme_Cover_Letter_v1.pdf` }),
    );
  });

  it('a per-row Generate click enqueues one job (U6)', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    await user.click(await screen.findByRole('button', { name: /generate letter for acme/i }));
    expect(api.generateLetters).toHaveBeenCalledTimes(1);
    expect(api.generateLetters).toHaveBeenCalledWith([7]);
  });

  it('R43 lead to approved PDF in ≤2 clicks without leaving Pipeline', async () => {
    const user = userEvent.setup();
    let clicks = 0;
    const click = async (el: Element): Promise<void> => {
      clicks++;
      await user.click(el);
    };
    const section = get(route);
    render(Pipeline);

    api.reviewQueue.mockResolvedValue([item(7, 'Acme', 'running')]);
    await click(await screen.findByRole('button', { name: /generate letter for acme/i }));
    const panel = await screen.findByRole('complementary', { name: /review/i });
    expect(await within(panel).findByText(/drafting/i)).toBeInTheDocument();

    // The background job finishes: Go reports a job change.
    api.reviewQueue.mockResolvedValue([item(7, 'Acme', 'draft')]);
    emit({ entity: 'job', ids: [7] });
    const entry = await within(panel).findByRole('listitem', { name: /acme/i });
    await click(await within(entry).findByRole('button', { name: /^approve$/i }));

    expect(api.approveLetter).toHaveBeenCalledWith(7);
    expect(await within(panel).findByText(/acme.*saved as version 1/i)).toBeInTheDocument();
    expect(clicks).toBeLessThanOrEqual(2);
    expect(get(route)).toBe(section);
    expect(screen.getByRole('heading', { name: 'Pipeline' })).toBeInTheDocument();
  });

  it('Approve all approves every draft and keeps failures with their reasons', async () => {
    const user = userEvent.setup();
    api.reviewQueue.mockResolvedValue([
      item(7, 'Acme', 'draft'),
      item(8, 'Globex', 'draft'),
      item(9, 'Initech', 'running'),
    ]);
    api.approveAll.mockResolvedValue({
      approved: [{ documentId: 1, appId: 7, version: 1, path: 'p' }],
      failed: [{ appId: 8, reason: 'The letter doesn’t fit on one page. Shorten it and try again.' }],
    });
    render(Pipeline);
    await user.click(await screen.findByRole('button', { name: /^review/i }));
    const panel = await screen.findByRole('complementary', { name: /review/i });
    await user.click(await within(panel).findByRole('button', { name: /approve all \(2\)/i }));

    expect(api.approveAll).toHaveBeenCalledWith([7, 8]);
    expect(await within(panel).findByText(/approved 1 letter/i)).toBeInTheDocument();
    expect(within(panel).getByText(/globex.*doesn.t fit on one page/i)).toBeInTheDocument();
  });

  it('a failed generation shows its reason and can be retried or dismissed', async () => {
    const user = userEvent.setup();
    api.reviewQueue.mockResolvedValue([item(8, 'Globex', 'failed', 'Gemini is busy right now. Try again in a minute.')]);
    render(Pipeline);
    await user.click(await screen.findByRole('button', { name: /^review/i }));
    const entry = await screen.findByRole('listitem', { name: /globex/i });
    expect(entry).toHaveTextContent(/gemini is busy/i);
    await user.click(within(entry).getByRole('button', { name: /retry/i }));
    expect(api.generateLetters).toHaveBeenCalledWith([8]);
    await user.click(within(entry).getByRole('button', { name: /dismiss/i }));
    expect(api.dismissJob).toHaveBeenCalledWith(8);
  });

  it('R26 the materials choice is set per row', async () => {
    const user = userEvent.setup();
    api.setMaterials.mockResolvedValue(row(7, 'Acme', 'lead', 'both'));
    render(Pipeline);
    const choice = await screen.findByRole('combobox', { name: /materials for acme/i });
    expect(choice).toHaveValue('letter');
    await user.selectOptions(choice, 'both');
    expect(api.setMaterials).toHaveBeenCalledWith(7, 'both');
  });
});
