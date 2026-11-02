import { describe, expect, it } from 'vitest';

import { filterRows, prune, toggle, type RowFilter } from './selection';

type Row = { id: number; company: string; role: string; stage: string };

const rows: Row[] = [
  { id: 1, company: 'Acme', role: 'Go developer', stage: 'applied' },
  { id: 2, company: 'Globex', role: 'Analyst', stage: 'lead' },
  { id: 3, company: 'Globex', role: 'Analyst', stage: 'applied' }, // twin of 2 by company+role
  { id: 4, company: '', role: 'Data engineer', stage: 'lead' },
  { id: 5, company: 'Łódź Soft', role: 'QA', stage: 'applied' },
];

const all: RowFilter = { stage: 'all', query: '' };

describe('R12 selection by ID', () => {
  it('filters by stage and by search text, case- and accent-insensitively', () => {
    expect(filterRows(rows, { stage: 'applied', query: '' }).map((r) => r.id)).toEqual([1, 3, 5]);
    expect(filterRows(rows, { stage: 'all', query: 'globex' }).map((r) => r.id)).toEqual([2, 3]);
    expect(filterRows(rows, { stage: 'all', query: 'lodz' }).map((r) => r.id)).toEqual([5]);
    expect(filterRows(rows, { stage: 'lead', query: 'analyst' }).map((r) => r.id)).toEqual([2]);
    expect(filterRows(rows, all)).toHaveLength(5);
  });

  it('toggle adds and removes an ID without touching its twin', () => {
    let s = toggle(new Set<number>(), 2);
    expect([...s]).toEqual([2]);
    s = toggle(s, 3);
    s = toggle(s, 2);
    expect([...s]).toEqual([3]);
  });

  it('prune keeps only visible IDs', () => {
    expect([...prune(new Set([1, 2, 3, 99]), [3, 1])].sort()).toEqual([1, 3]);
  });
});
