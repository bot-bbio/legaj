import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { StartupView } from '../lib/api';

const api = {
  startup: vi.fn<() => Promise<StartupView>>(),
  restoreBackup: vi.fn<(name: string) => Promise<StartupView>>(),
  quit: vi.fn<() => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: StartupGate } = await import('./StartupGate.svelte');

const children = createRawSnippet(() => ({ render: () => '<p>the app</p>' }));

const backups = [
  { name: 'legaj-20261101T090000Z.db', takenAt: '2026-11-01T09:00:00Z', sizeBytes: 40960 },
  { name: 'legaj-20261031T090000Z.db', takenAt: '2026-10-31T09:00:00Z', sizeBytes: 40960 },
];

describe('R2 startup states', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the app when the database is ready', async () => {
    api.startup.mockResolvedValue({ kind: 'ready', movedAside: '', backups: [], message: '' });
    render(StartupGate, { children });
    expect(await screen.findByText('the app')).toBeInTheDocument();
  });

  it('offers verified backups after corruption and restores the chosen one', async () => {
    api.startup.mockResolvedValue({ kind: 'corrupt', movedAside: 'legaj.db.corrupt-20261101T100000Z', backups, message: '' });
    api.restoreBackup.mockResolvedValue({ kind: 'ready', movedAside: '', backups: [], message: '' });
    render(StartupGate, { children });

    expect(await screen.findByRole('heading', { name: /damaged/i })).toBeInTheDocument();
    expect(screen.getByText(/legaj\.db\.corrupt-20261101T100000Z/)).toBeInTheDocument();
    expect(screen.queryByText('the app')).not.toBeInTheDocument();

    const restoreButtons = screen.getAllByRole('button', { name: /restore/i });
    expect(restoreButtons).toHaveLength(2);
    const first = restoreButtons[0];
    if (!first) throw new Error('no restore button');
    await userEvent.click(first);

    expect(api.restoreBackup).toHaveBeenCalledWith('legaj-20261101T090000Z.db');
    expect(await screen.findByText('the app')).toBeInTheDocument();
  });

  it('offers only Quit when there is no usable backup', async () => {
    api.startup.mockResolvedValue({ kind: 'migration_failed', movedAside: '', backups: [], message: '' });
    render(StartupGate, { children });

    expect(await screen.findByText(/no usable backup/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /restore/i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /quit/i }));
    expect(api.quit).toHaveBeenCalled();
  });

  it('a missing data file with backups offers restore, never a silent fresh start', async () => {
    api.startup.mockResolvedValue({ kind: 'missing', movedAside: '', backups, message: '' });
    render(StartupGate, { children });
    expect(await screen.findByRole('heading', { name: /couldn.t find your data/i })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /restore/i })).toHaveLength(2);
    expect(screen.queryByText('the app')).not.toBeInTheDocument();
  });

  it('shows the backend message for a startup error', async () => {
    api.startup.mockResolvedValue({ kind: 'error', movedAside: '', backups: [], message: 'LeGaJ couldn’t open its database.' });
    render(StartupGate, { children });
    expect(await screen.findByText('LeGaJ couldn’t open its database.')).toBeInTheDocument();
  });
});
