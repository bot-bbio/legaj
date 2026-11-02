/**
 * Row filtering and selection by stable ID (R12). Nothing here uses list
 * positions: V1 selected by index, so a delete shifted the indexes and the
 * next bulk action hit the wrong rows (U2, U4, C2).
 */

export interface RowFilter {
  stage: string; // a stage id, or 'all'
  query: string;
}

interface Filterable {
  id: number;
  company: string;
  role: string;
  stage: string;
}

/** Letters that NFD doesn't decompose into base + accent. */
const FOLD: Record<string, string> = { ł: 'l', đ: 'd', ø: 'o', ß: 'ss', æ: 'ae', œ: 'oe' };

/** Lower-cases and strips accents, so "lodz" finds "Łódź". */
export function fold(s: string): string {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .replace(/[łđøßæœ]/g, (c) => FOLD[c] ?? c);
}

export function filterRows<T extends Filterable>(rows: readonly T[], f: RowFilter): T[] {
  const q = fold(f.query.trim());
  return rows.filter(
    (r) => (f.stage === 'all' || r.stage === f.stage) && (q === '' || fold(`${r.company} ${r.role}`).includes(q)),
  );
}

export function toggle(selected: ReadonlySet<number>, id: number): Set<number> {
  const next = new Set(selected);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}

/** Keeps only the selected IDs that are still visible. */
export function prune(selected: ReadonlySet<number>, visibleIds: readonly number[]): Set<number> {
  const visible = new Set(visibleIds);
  return new Set([...selected].filter((id) => visible.has(id)));
}
