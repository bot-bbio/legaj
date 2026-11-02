import { fireEvent, render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApplicationRow } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const row = (id: number, company: string, stage: string): ApplicationRow => ({
  id,
  company,
  role: `Role ${id}`,
  location: '',
  url: '',
  stage,
  source: 'paste',
  createdAt: '2026-11-01T09:00:00Z',
  appliedAt: '',
});

const api = { listApplications: vi.fn<() => Promise<ApplicationRow[]>>() };
vi.mock('../lib/api', () => ({ api }));

const { default: Pipeline } = await import('./Pipeline.svelte');

function selectedIds(): number[] {
  return screen
    .getAllByRole('checkbox', { name: /^select /i })
    .filter((c) => c.hasAttribute('data-id') && (c as HTMLInputElement).checked)
    .map((c) => Number(c.getAttribute('data-id')));
}

describe('R12 Pipeline selection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listApplications.mockResolvedValue([
      row(1, 'Acme', 'applied'),
      row(2, 'Globex', 'lead'),
      row(3, 'Globex', 'applied'),
      row(4, 'Initech', 'lead'),
      row(5, 'Umbrella', 'applied'),
    ]);
  });

  it('R12 select all respects filter: exactly the N visible IDs are selected (U2)', async () => {
    render(Pipeline);
    await screen.findByText('Acme');
    await userEvent.selectOptions(screen.getByLabelText(/stage/i), 'applied');
    expect(screen.getByText(/3 of 5 shown/i)).toBeInTheDocument();

    await userEvent.click(screen.getByRole('checkbox', { name: /select all shown/i }));
    expect(selectedIds().sort()).toEqual([1, 3, 5]);
    expect(screen.getByText(/3 selected/i)).toBeInTheDocument();

    // Narrowing further drops selections that are no longer visible.
    await userEvent.type(screen.getByLabelText(/search/i), 'globex');
    expect(selectedIds()).toEqual([3]);
    expect(screen.getByText(/1 selected/i)).toBeInTheDocument();
  });

  it('a row checkbox selects that ID only, not its company+role twin', async () => {
    render(Pipeline);
    await screen.findByText('Acme');
    const twin = screen.getAllByRole('checkbox', { name: /^select /i }).find((c) => c.getAttribute('data-id') === '3');
    if (!twin) throw new Error('row 3 missing');
    await userEvent.click(twin);
    expect(selectedIds()).toEqual([3]);
  });

  it('rows without a company show "Needs company"', async () => {
    api.listApplications.mockResolvedValue([row(9, '', 'lead')]);
    render(Pipeline);
    expect(await screen.findByText(/needs company/i)).toBeInTheDocument();
  });
});

describe('R17 Pipeline at scale', () => {
  const many = Array.from({ length: 1000 }, (_, i) => row(i + 1, `Company ${i + 1}`, i % 2 ? 'lead' : 'applied'));

  beforeEach(() => {
    api.listApplications.mockResolvedValue(many);
  });

  it('R17 1000 rows renders <=100 row nodes, within 1 s', async () => {
    const t0 = performance.now();
    render(Pipeline);
    await screen.findByText('Company 1');
    const elapsed = performance.now() - t0;

    const table = screen.getByRole('table');
    expect(table).toHaveAttribute('aria-rowcount', '1001');
    expect(within(table).getAllByRole('row').length).toBeLessThanOrEqual(100);
    expect(elapsed).toBeLessThan(1000);
  });

  it('scrolling renders the rows in view', async () => {
    render(Pipeline);
    await screen.findByText('Company 1');
    const viewport = screen.getByTestId('virtual-viewport');
    viewport.scrollTop = 500 * 36;
    await fireEvent.scroll(viewport);
    expect(await screen.findByText('Company 501')).toBeInTheDocument();
    expect(screen.queryByText('Company 1')).not.toBeInTheDocument();
  });

  it('select all with 1000 rows selects all 1000 by ID', async () => {
    render(Pipeline);
    await screen.findByText('Company 1');
    await userEvent.click(screen.getByRole('checkbox', { name: /select all shown/i }));
    expect(screen.getByText(/1000 selected/i)).toBeInTheDocument();
  });
});
