import type { YearMonth } from './api';

/** Mirrors Go's profile.ParseYearMonth: "YYYY-MM", "YYYY", or "" (unset). */
export function parseYearMonth(text: string): YearMonth | null {
  const s = text.trim();
  if (s === '') return { year: 0, month: 0 };
  const m = /^(\d{4})(?:-(\d{2}))?$/.exec(s);
  if (!m) return null;
  const year = Number(m[1]);
  const month = m[2] === undefined ? 0 : Number(m[2]);
  if (year < 1800 || year > 2100 || month > 12 || (m[2] !== undefined && month < 1)) return null;
  return { year, month };
}

export function formatYearMonth(d: YearMonth): string {
  if (d.year === 0 && d.month === 0) return '';
  const y = String(d.year).padStart(4, '0');
  return d.month === 0 ? y : `${y}-${String(d.month).padStart(2, '0')}`;
}
