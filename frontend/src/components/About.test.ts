import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

const api = {
  version: vi.fn<() => Promise<string>>(() => Promise.resolve('2.0.0')),
  openURL: vi.fn<(u: string) => Promise<void>>(() => Promise.resolve()),
};
vi.mock('../lib/api', () => ({ api }));

const { default: About } = await import('./About.svelte');

describe('R41 about line', () => {
  it('shows the version, the license and the author credit', async () => {
    const user = userEvent.setup();
    render(About);
    expect(await screen.findByText(/legaj 2\.0\.0/i)).toBeInTheDocument();
    expect(screen.getByText(/mit license/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /roberto montero/i }));
    expect(api.openURL).toHaveBeenCalledWith('https://linkedin.com/in/alvvays');
  });
});
