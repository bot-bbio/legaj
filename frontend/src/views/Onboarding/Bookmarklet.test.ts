import { createEvent, fireEvent, render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ClipStatus } from '../../lib/api';

vi.mock('../../lib/events', () => ({ onEntityChanged: () => () => {} }));

const status = (over: Partial<ClipStatus> = {}): ClipStatus => ({
  running: true,
  port: 47123,
  needsReinstall: false,
  message: '',
  ...over,
});

const api = {
  clipStatus: vi.fn<() => Promise<ClipStatus>>(),
  copyText: vi.fn<(text: string) => Promise<void>>(),
};
vi.mock('../../lib/api', () => ({ api }));

const { default: Bookmarklet } = await import('./Bookmarklet.svelte');

const link = (): Promise<HTMLElement> => screen.findByRole('link', { name: /clip to legaj/i });

describe('R18 install page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.clipStatus.mockResolvedValue(status());
    api.copyText.mockResolvedValue();
  });

  it('offers a bookmarklet for the persisted port', async () => {
    render(Bookmarklet);
    const href = (await link()).getAttribute('href') ?? '';
    expect(href.startsWith('javascript:')).toBe(true);
    expect(decodeURIComponent(href)).toContain('http://127.0.0.1:47123');
  });

  it('follows the port: a moved server gets a new bookmarklet', async () => {
    api.clipStatus.mockResolvedValue(status({ port: 47999, needsReinstall: true }));
    render(Bookmarklet);
    expect(decodeURIComponent((await link()).getAttribute('href') ?? '')).toContain('http://127.0.0.1:47999');
  });

  // Clicked here, it would run on LeGaJ's own window, not on a job page.
  it('does nothing when clicked: it belongs in the browser', async () => {
    render(Bookmarklet);
    const el = await link();
    const click = createEvent.click(el);
    await fireEvent(el, click);
    expect(click.defaultPrevented).toBe(true);
  });

  it('offers a copy for when dragging out of LeGaJ does not work', async () => {
    render(Bookmarklet);
    const href = (await link()).getAttribute('href');
    await userEvent.click(screen.getByRole('button', { name: /copy/i }));
    expect(api.copyText).toHaveBeenCalledExactlyOnceWith(href);
    expect(await screen.findByText(/copied/i)).toBeInTheDocument();
  });

  it('says so when the copy fails', async () => {
    api.copyText.mockRejectedValue('LeGaJ couldn’t copy to the clipboard.');
    render(Bookmarklet);
    await link();
    await userEvent.click(screen.getByRole('button', { name: /copy/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent('couldn’t copy');
    expect(screen.queryByText(/copied/i)).not.toBeInTheDocument();
  });

  it('offers no bookmarklet while the clipper is not running, and says why', async () => {
    api.clipStatus.mockResolvedValue(status({ running: false, port: 0, message: 'No free local port was found.' }));
    render(Bookmarklet);
    expect(await screen.findByText('No free local port was found.')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copy/i })).not.toBeInTheDocument();
  });

  it('still says something when there is no reason to give', async () => {
    api.clipStatus.mockResolvedValue(status({ running: false, port: 0 }));
    render(Bookmarklet);
    expect(await screen.findByText(/restart legaj/i)).toBeInTheDocument();
  });

  it('shows why when the status can not be read', async () => {
    api.clipStatus.mockRejectedValue('LeGaJ couldn’t read its clipper settings.');
    render(Bookmarklet);
    expect(await screen.findByText('LeGaJ couldn’t read its clipper settings.')).toBeInTheDocument();
  });

  it('explains that LeGaJ must be running and points to Start with Windows', async () => {
    render(Bookmarklet);
    await link();
    const page = document.body.textContent;
    expect(page).toMatch(/has to be running/i);
    expect(page).toMatch(/start legaj with windows/i);
  });

  it('shows the bookmarks bar shortcut (R7)', async () => {
    render(Bookmarklet);
    await link();
    expect(document.body.textContent).toContain('Ctrl+Shift+B');
  });

  // Saving a clip can send page text to Google with no further click (D31).
  it('says when a clip reaches Gemini, and what it sends', async () => {
    render(Bookmarklet);
    await link();
    const page = document.body.textContent.replace(/\s+/g, ' ');
    expect(page).toMatch(/missing its company or role/i);
    expect(page).toMatch(/LeGaJ asks Gemini, sending the page’s title and the saved description/i);
  });

  it('says that selecting the description helps on unknown pages (R19)', async () => {
    render(Bookmarklet);
    await link();
    expect(document.body.textContent).toMatch(/select the job description/i);
  });
});
