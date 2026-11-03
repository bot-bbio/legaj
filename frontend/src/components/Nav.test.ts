import { render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { DueView } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const due = (appId: number): DueView => ({
  kind: 'applied_nudge',
  appId,
  roundId: 0,
  company: `Co ${appId}`,
  role: 'Dev',
  interviewer: '',
  dueSince: '2026-11-02T00:00:00Z',
  daysSince: 7,
});

const api = { followupsDue: vi.fn<() => Promise<DueView[]>>() };
vi.mock('../lib/api', () => ({ api }));

const { default: Nav } = await import('./Nav.svelte');

describe('R28 nav badge', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.clearAllMocks();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows the due count on Follow-ups, and refreshes as time passes', async () => {
    api.followupsDue.mockResolvedValue([due(1), due(2)]);
    render(Nav, { active: 'pipeline', onselect: () => {} });
    expect(await screen.findByRole('button', { name: /follow-ups, 2 due/i })).toBeInTheDocument();

    // Nothing changed in the DB, but a round ended: the next minute's refresh shows it.
    api.followupsDue.mockResolvedValue([due(1), due(2), due(3)]);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(await screen.findByRole('button', { name: /follow-ups, 3 due/i })).toBeInTheDocument();
  });

  it('shows no badge when nothing is due', async () => {
    api.followupsDue.mockResolvedValue([]);
    render(Nav, { active: 'pipeline', onselect: () => {} });
    await vi.advanceTimersByTimeAsync(10);
    expect(screen.getByRole('button', { name: /^follow-ups$/i })).toBeInTheDocument();
    expect(screen.getAllByRole('button')).toHaveLength(5);
  });
});
