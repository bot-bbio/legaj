import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { UpdateResult } from '../lib/api';

const api = {
  checkForUpdate: vi.fn<() => Promise<UpdateResult>>(),
  openURL: vi.fn<(u: string) => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: UpdateBanner } = await import('./UpdateBanner.svelte');

const url = 'https://github.com/bot-bbio/legaj/releases/tag/v2.1.0';

describe('R40 update banner', () => {
  beforeEach(() => vi.clearAllMocks());

  it('offers Download for a newer release and never installs anything', async () => {
    const user = userEvent.setup();
    api.checkForUpdate.mockResolvedValue({ available: true, latest: '2.1.0', url });
    render(UpdateBanner);
    expect(await screen.findByText(/legaj 2\.1\.0 is available/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /download/i }));
    expect(api.openURL).toHaveBeenCalledWith(url);
  });

  it('can be dismissed', async () => {
    const user = userEvent.setup();
    api.checkForUpdate.mockResolvedValue({ available: true, latest: '2.1.0', url });
    render(UpdateBanner);
    await user.click(await screen.findByRole('button', { name: /not now/i }));
    expect(screen.queryByText(/is available/i)).not.toBeInTheDocument();
  });

  it('shows nothing when up to date or when the check fails', async () => {
    api.checkForUpdate.mockResolvedValue({ available: false, latest: '2.0.0', url: '' });
    const { unmount } = render(UpdateBanner);
    await Promise.resolve();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    unmount();
    api.checkForUpdate.mockRejectedValue('offline');
    render(UpdateBanner);
    await Promise.resolve();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
