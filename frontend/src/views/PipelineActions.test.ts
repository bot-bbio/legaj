import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApplicationRow, DeleteResult, StageResult, StatsView, UndoResult } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const row = (id: number, company: string, stage: string): ApplicationRow => ({
  id, company, role: `Role ${id}`, location: '', url: '', stage, source: 'paste', materials: 'letter', letterVersion: 0,
  createdAt: '2026-11-01T09:00:00Z', appliedAt: '',
});

const stats: StatsView = {
  counts: { lead: 2, ready: 0, applied: 1, interviewing: 0, offer: 0, rejected: 0, withdrawn: 0 },
  total: 3,
  recent: [
    { id: 3, company: 'Initech', role: 'QA', stage: 'lead', createdAt: '2026-11-01T09:00:00Z' },
    { id: 2, company: 'Globex', role: 'Analyst', stage: 'lead', createdAt: '2026-10-31T09:00:00Z' },
  ],
  nextInterview: { applicationId: 1, company: 'Acme', role: 'Dev', startsAt: '2026-11-03T14:00:00Z', zone: 'Europe/Warsaw', kind: 'video' },
};

const api = {
  listApplications: vi.fn<() => Promise<ApplicationRow[]>>(),
  setStage: vi.fn<(ids: number[], stage: string) => Promise<StageResult>>(),
  deleteApplications: vi.fn<(ids: number[]) => Promise<DeleteResult>>(),
  undoDelete: vi.fn<(id: string) => Promise<UndoResult>>(),
  reopen: vi.fn<(ids: number[], stage: string) => Promise<StageResult>>(),
  pipelineStats: vi.fn<() => Promise<StatsView>>(),
  openPosting: vi.fn<(id: number) => Promise<void>>(() => Promise.resolve()),
  openResume: vi.fn<(id: number) => Promise<void>>(() => Promise.reject('No resume has been uploaded yet.')),
};
vi.mock('../lib/api', () => ({ api }));

const { default: Pipeline } = await import('./Pipeline.svelte');

async function selectRows(user: ReturnType<typeof userEvent.setup>, ...ids: number[]): Promise<void> {
  await screen.findByRole('checkbox', { name: /select acme/i });
  for (const id of ids) {
    const box = screen.getAllByRole('checkbox').find((c) => c.getAttribute('data-id') === String(id));
    if (!box) throw new Error(`row ${id} missing`);
    await user.click(box);
  }
}

describe('R13 delete and undo', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([row(1, 'Acme', 'applied'), row(2, 'Globex', 'lead'), row(3, 'Initech', 'lead')]);
    api.pipelineStats.mockResolvedValue(stats);
    api.deleteApplications.mockImplementation((ids) => Promise.resolve({ undoId: 'u1', ids: [...ids].sort() }));
    api.undoDelete.mockResolvedValue({ ids: [1, 3], skipped: [], mergedInto: [] });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('the undo toast calls undo within 10 s', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 1, 3);
    await user.click(screen.getByRole('button', { name: /^delete/i }));

    expect(api.deleteApplications).toHaveBeenCalledTimes(1);
    expect([...(api.deleteApplications.mock.calls[0]?.[0] ?? [])].sort()).toEqual([1, 3]);
    const toast = await screen.findByRole('status');
    expect(toast).toHaveTextContent(/deleted 2 applications/i);

    await vi.advanceTimersByTimeAsync(9000);
    await user.click(within(toast).getByRole('button', { name: /undo/i }));
    expect(api.undoDelete).toHaveBeenCalledWith('u1');
  });

  it('the toast closes after 10 s, after which undo is gone', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 2);
    await user.click(screen.getByRole('button', { name: /^delete/i }));
    expect(await screen.findByRole('status')).toHaveTextContent(/deleted 1 application\b/i);

    await vi.advanceTimersByTimeAsync(10_000);
    expect(screen.queryByRole('button', { name: /undo/i })).not.toBeInTheDocument();
    expect(api.undoDelete).not.toHaveBeenCalled();
  });

  it('closes before the backend purges, so Undo is never offered when it can no longer work (gate CR7)', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 2);
    await user.click(screen.getByRole('button', { name: /^delete/i }));
    await screen.findByRole('status');
    await vi.advanceTimersByTimeAsync(9_600);
    expect(screen.queryByRole('button', { name: /undo/i })).not.toBeInTheDocument();
  });

  it('a second delete keeps the first one undoable (gate C8)', async () => {
    let n = 0;
    api.deleteApplications.mockImplementation((ids) => Promise.resolve({ undoId: `u${++n}`, ids: [...ids] }));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 1);
    await user.click(screen.getByRole('button', { name: /^delete/i }));
    await selectRows(user, 2);
    await user.click(screen.getByRole('button', { name: /^delete/i }));

    const undos = await screen.findAllByRole('button', { name: /undo/i });
    expect(undos).toHaveLength(2);
    const first = undos[0];
    if (!first) throw new Error('missing');
    await user.click(first);
    expect(api.undoDelete).toHaveBeenCalledWith('u1');
  });

  it('keeps the partial-restore note until closed (gate CR7)', async () => {
    api.undoDelete.mockResolvedValue({ ids: [1], skipped: [3], mergedInto: [7] });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 1, 3);
    await user.click(screen.getByRole('button', { name: /^delete/i }));
    await user.click(within(await screen.findByRole('status')).getByRole('button', { name: /undo/i }));
    expect(await screen.findByText(/moved to the job you added again/i)).toBeInTheDocument();
    await vi.advanceTimersByTimeAsync(15_000);
    expect(screen.getByText(/moved to the job you added again/i)).toBeInTheDocument();
  });

  it('reports rows Undo could not bring back', async () => {
    api.undoDelete.mockResolvedValue({ ids: [1], skipped: [3], mergedInto: [7] });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(Pipeline);
    await selectRows(user, 1, 3);
    await user.click(screen.getByRole('button', { name: /^delete/i }));
    await user.click(within(await screen.findByRole('status')).getByRole('button', { name: /undo/i }));
    expect(await screen.findByText(/1 couldn.t be restored/i)).toBeInTheDocument();
  });
});

describe('R11 Reopen from the bulk bar (gate C4)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([row(1, 'Acme', 'rejected'), row(2, 'Globex', 'lead')]);
    api.pipelineStats.mockResolvedValue(stats);
    api.setStage.mockResolvedValue({ changed: [], skipped: [{ id: 1, reason: 'Can’t move from Rejected to Applied. Use Reopen to move a row back.' }] });
    api.reopen.mockResolvedValue({ changed: [1], skipped: [] });
  });

  it('drops the Reopen offer when the selection changes, and never offers it for a terminal target', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    await selectRows(user, 1);
    await user.selectOptions(screen.getByLabelText(/move selected to/i), 'applied');
    await user.click(screen.getByRole('button', { name: /^move$/i }));
    expect(await screen.findByRole('button', { name: /reopen 1 to applied/i })).toBeInTheDocument();
    await selectRows(user, 2); // the selection is now {1, 2}
    expect(screen.queryByRole('button', { name: /reopen/i })).not.toBeInTheDocument();

    api.setStage.mockResolvedValue({ changed: [], skipped: [{ id: 1, reason: 'Can’t move from Rejected to Withdrawn.' }] });
    await user.selectOptions(screen.getByLabelText(/move selected to/i), 'withdrawn');
    await user.click(screen.getByRole('button', { name: /^move$/i }));
    await screen.findByText(/rejected to withdrawn/i);
    expect(screen.queryByRole('button', { name: /reopen/i })).not.toBeInTheDocument();
  });

  it('offers Reopen for rows the move refused, with the same target', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    await selectRows(user, 1);
    await user.selectOptions(screen.getByLabelText(/move selected to/i), 'applied');
    await user.click(screen.getByRole('button', { name: /^move$/i }));
    await user.click(await screen.findByRole('button', { name: /reopen 1 to applied/i }));
    expect(api.reopen).toHaveBeenCalledWith([1], 'applied');
  });
});

describe('R12 bulk stage change', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([row(1, 'Acme', 'offer'), row(2, 'Globex', 'lead'), row(3, 'Initech', 'lead')]);
    api.pipelineStats.mockResolvedValue(stats);
    api.setStage.mockResolvedValue({ changed: [2], skipped: [{ id: 1, reason: 'Can’t move from Offer to Lead. Use Reopen to move a row back.' }] });
  });

  it('moves exactly the selected IDs and reports refused rows', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    await selectRows(user, 1, 2);
    await user.selectOptions(screen.getByLabelText(/move selected to/i), 'lead');
    await user.click(screen.getByRole('button', { name: /^move$/i }));

    const [ids, stage] = api.setStage.mock.calls[0] ?? [[], ''];
    expect([...ids].sort()).toEqual([1, 2]);
    expect(stage).toBe('lead');
    expect(await screen.findByText(/1 moved/i)).toBeInTheDocument();
    expect(screen.getByText(/use reopen/i)).toBeInTheDocument();
  });
});

describe('R33/R10 row links', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([{ ...row(1, 'Acme', 'applied'), url: 'https://jobs.lever.co/acme/1' }, row(2, 'Globex', 'lead')]);
    api.pipelineStats.mockResolvedValue(stats);
  });

  it('opens the posting through the backend, and only for rows that have one', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    const acme = (await screen.findByRole('checkbox', { name: /select acme/i })).closest('[role="row"]');
    if (!(acme instanceof HTMLElement)) throw new Error('row missing');
    await user.click(within(acme).getByRole('button', { name: /posting/i }));
    expect(api.openPosting).toHaveBeenCalledWith(1);

    const globex = screen.getByRole('checkbox', { name: /select globex/i }).closest('[role="row"]');
    if (!(globex instanceof HTMLElement)) throw new Error('row missing');
    expect(within(globex).queryByRole('button', { name: /posting/i })).not.toBeInTheDocument();
  });

  it('shows why the resume could not be opened', async () => {
    const user = userEvent.setup();
    render(Pipeline);
    const acme = (await screen.findByRole('checkbox', { name: /select acme/i })).closest('[role="row"]');
    if (!(acme instanceof HTMLElement)) throw new Error('row missing');
    await user.click(within(acme).getByRole('button', { name: /resume/i }));
    expect(api.openResume).toHaveBeenCalledWith(1);
    expect(await screen.findByRole('alert')).toHaveTextContent('No resume has been uploaded yet.');
  });
});

describe('R14 stats header', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([row(1, 'Acme', 'applied')]);
    api.pipelineStats.mockResolvedValue(stats);
  });

  it('shows per-stage counts, the newest applications and the next interview', async () => {
    render(Pipeline);
    const header = await screen.findByRole('region', { name: /pipeline summary/i });
    expect(within(header).getByText(/lead/i).closest('li')).toHaveTextContent('2');
    const recent = within(header).getByRole('list', { name: /recently added/i });
    expect(within(recent).getAllByRole('listitem').map((li) => li.textContent)).toEqual([
      expect.stringContaining('Initech'),
      expect.stringContaining('Globex'),
    ]);
    expect(within(header).getByText(/next interview/i).parentElement).toHaveTextContent(/Acme/);
  });

  it('says so when no interview is scheduled', async () => {
    api.pipelineStats.mockResolvedValue({ counts: stats.counts, total: stats.total, recent: stats.recent });
    render(Pipeline);
    expect(await screen.findByText(/no interviews scheduled/i)).toBeInTheDocument();
  });
});
