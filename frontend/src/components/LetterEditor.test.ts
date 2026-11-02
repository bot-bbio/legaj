import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { DocumentVersion, Draft, DraftPart } from '../lib/api';

const draft = (): Draft => ({
  style: 'direct',
  salutation: 'Dear Hiring Team,',
  opening: 'I am applying to Acme.',
  whyOptions: ['Why A about Acme.', 'Why B about Acme.', 'Why C about Acme.'],
  chosenWhy: 0,
  body: ['At Babbage & Co I wrote the first algorithm.'],
  closing: 'I would welcome a conversation.',
  signOff: 'Sincerely,',
});

const api = {
  getDraft: vi.fn<(id: number) => Promise<Draft>>(),
  saveDraft: vi.fn<(id: number, d: Draft) => Promise<Draft>>((_id, d) => Promise.resolve(d)),
  regenerateParagraph: vi.fn<(id: number, p: DraftPart, i: number) => Promise<Draft>>(),
  approveLetter: vi.fn<(id: number) => Promise<DocumentVersion>>(),
};
vi.mock('../lib/api', () => ({ api }));

const { default: LetterEditor } = await import('./LetterEditor.svelte');

function lastSaved(): Draft {
  const c = api.saveDraft.mock.calls.at(-1);
  if (!c) throw new Error('saveDraft not called');
  return c[1];
}

describe('R22 letter review editor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getDraft.mockResolvedValue(draft());
    api.approveLetter.mockResolvedValue({ documentId: 1, appId: 5, version: 1, path: 'documents/5_Acme/Acme_Cover_Letter_v1.pdf' });
  });

  it('picking a why-option updates the draft', async () => {
    render(LetterEditor, { appId: 5 });
    const why = await screen.findByRole('radiogroup', { name: /why this company/i });
    await userEvent.click(within(why).getByRole('radio', { name: /why c about acme/i }));
    expect(api.saveDraft).toHaveBeenCalled();
    expect(lastSaved().chosenWhy).toBe(2);
  });

  it('regenerating a paragraph replaces only that paragraph on screen', async () => {
    const regenerated = { ...draft(), body: ['A fresh body paragraph about Acme.'] };
    api.regenerateParagraph.mockResolvedValue(regenerated);
    render(LetterEditor, { appId: 5 });
    await screen.findByDisplayValue('I am applying to Acme.');
    await userEvent.click(screen.getByRole('button', { name: /regenerate body paragraph 1/i }));
    expect(api.regenerateParagraph).toHaveBeenCalledWith(5, 'body', 0);
    expect(await screen.findByDisplayValue('A fresh body paragraph about Acme.')).toBeInTheDocument();
    expect(screen.getByDisplayValue('I am applying to Acme.')).toBeInTheDocument();
  });

  it('Approve saves edits first, then makes the PDF version', async () => {
    const onapproved = vi.fn();
    render(LetterEditor, { appId: 5, onapproved });
    const opening = await screen.findByDisplayValue('I am applying to Acme.');
    await userEvent.clear(opening);
    await userEvent.type(opening, 'I am excited to apply to Acme.');
    await userEvent.click(screen.getByRole('button', { name: /approve/i }));

    expect(lastSaved().opening).toBe('I am excited to apply to Acme.');
    expect(api.approveLetter).toHaveBeenCalledWith(5);
    const saveOrder = api.saveDraft.mock.invocationCallOrder.at(-1) ?? 0;
    const approveOrder = api.approveLetter.mock.invocationCallOrder[0] ?? 0;
    expect(saveOrder).toBeLessThan(approveOrder);
    expect(await screen.findByText(/saved as version 1/i)).toBeInTheDocument();
    expect(onapproved).toHaveBeenCalledWith(expect.objectContaining({ version: 1 }));
  });

  it('shows the backend reason when approval fails', async () => {
    api.approveLetter.mockRejectedValue('The letter doesn’t fit on one page. Shorten it and try again.');
    render(LetterEditor, { appId: 5 });
    await screen.findByDisplayValue('I am applying to Acme.');
    await userEvent.click(screen.getByRole('button', { name: /approve/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/doesn.t fit on one page/);
  });
});
