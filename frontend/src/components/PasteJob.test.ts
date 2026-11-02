import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PasteInput, PasteResult } from '../lib/api';

const api = { pasteJob: vi.fn<(i: PasteInput) => Promise<PasteResult>>() };
vi.mock('../lib/api', () => ({ api }));

const { default: PasteJob } = await import('./PasteJob.svelte');

describe('R15 Paste job', () => {
  beforeEach(() => vi.clearAllMocks());

  async function fill(): Promise<void> {
    await userEvent.type(screen.getByLabelText(/job url/i), 'https://jobs.lever.co/acme/abc');
    await userEvent.type(screen.getByLabelText(/company/i), 'Acme');
    await userEvent.type(screen.getByLabelText(/role/i), 'Go developer');
    await userEvent.type(screen.getByLabelText(/job description/i), 'We build things.');
  }

  it('saves the entered job as a Lead and closes', async () => {
    api.pasteJob.mockResolvedValue({ created: true, id: 5, duplicateOf: 0 });
    const onclose = vi.fn();
    render(PasteJob, { onclose, onshow: vi.fn() });
    await fill();
    await userEvent.click(screen.getByRole('button', { name: /add to pipeline/i }));

    expect(api.pasteJob).toHaveBeenCalledWith({
      url: 'https://jobs.lever.co/acme/abc',
      company: 'Acme',
      role: 'Go developer',
      location: '',
      description: 'We build things.',
    });
    expect(onclose).toHaveBeenCalled();
  });

  it('offers the existing row when the job is already in the pipeline', async () => {
    api.pasteJob.mockResolvedValue({ created: false, id: 3, duplicateOf: 3 });
    const onshow = vi.fn();
    const onclose = vi.fn();
    render(PasteJob, { onclose, onshow });
    await fill();
    await userEvent.click(screen.getByRole('button', { name: /add to pipeline/i }));

    expect(await screen.findByText(/already in your pipeline/i)).toBeInTheDocument();
    expect(onclose).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: /show it/i }));
    expect(onshow).toHaveBeenCalledWith(3);
  });

  it('shows a validation message from the backend', async () => {
    api.pasteJob.mockRejectedValue('Enter at least the company or the role.');
    render(PasteJob, { onclose: vi.fn(), onshow: vi.fn() });
    await userEvent.type(screen.getByLabelText(/job url/i), 'https://example.com/j/1');
    await userEvent.click(screen.getByRole('button', { name: /add to pipeline/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Enter at least the company or the role.');
  });
});
