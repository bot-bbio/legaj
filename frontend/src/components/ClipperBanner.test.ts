import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ClipStatus } from '../lib/api';
import type { EntityChange } from '../lib/events';

const listeners = vi.hoisted(() => new Set<(c: EntityChange) => void>());
vi.mock('../lib/events', () => ({
  onEntityChanged: (cb: (c: EntityChange) => void) => {
    listeners.add(cb);
    return () => listeners.delete(cb);
  },
}));

const status = (over: Partial<ClipStatus> = {}): ClipStatus => ({
  running: true,
  port: 47123,
  needsReinstall: false,
  message: '',
  ...over,
});

const api = { clipStatus: vi.fn<() => Promise<ClipStatus>>() };
vi.mock('../lib/api', () => ({ api }));

const { default: ClipperBanner } = await import('./ClipperBanner.svelte');
const { navigate, route } = await import('../lib/stores/route');

describe('R18 clipper banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigate('profile');
  });

  it('stays out of the way when the clipper is fine', async () => {
    api.clipStatus.mockResolvedValue(status());
    render(ClipperBanner);
    await waitFor(() => expect(api.clipStatus).toHaveBeenCalled());
    await Promise.resolve();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('links to the install page when the port moved', async () => {
    api.clipStatus.mockResolvedValue(status({ needsReinstall: true }));
    render(ClipperBanner);
    await userEvent.click(await screen.findByRole('button', { name: /reinstall bookmarklet/i }));
    expect(get(route)).toBe('settings');
  });

  it('clears once the bookmarklet is reinstalled, without a restart', async () => {
    api.clipStatus.mockResolvedValue(status({ needsReinstall: true }));
    render(ClipperBanner);
    const button = await screen.findByRole('button', { name: /reinstall bookmarklet/i });

    api.clipStatus.mockResolvedValue(status());
    for (const listener of listeners) listener({ entity: 'settings', ids: [] });
    await waitFor(() => expect(button).not.toBeInTheDocument());
  });

  it('shows why when the clipper could not start', async () => {
    api.clipStatus.mockResolvedValue(status({ running: false, port: 0, message: 'No free local port was found.' }));
    render(ClipperBanner);
    expect(await screen.findByRole('alert')).toHaveTextContent('No free local port was found.');
    expect(screen.queryByRole('button', { name: /reinstall/i })).not.toBeInTheDocument();
  });

  it('stays quiet when the status can not be read', async () => {
    api.clipStatus.mockRejectedValue('nope');
    render(ClipperBanner);
    await waitFor(() => expect(api.clipStatus).toHaveBeenCalled());
    await Promise.resolve();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
