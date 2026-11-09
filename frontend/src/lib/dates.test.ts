import { describe, expect, it } from 'vitest';

import { formatYearMonth, parseYearMonth } from './dates';

describe('R9 year-month dates (A5)', () => {
  it('round-trips without splitting on "-"', () => {
    for (const s of ['1842-01', '1843-09', '2026', '']) {
      const d = parseYearMonth(s);
      expect(d).not.toBeNull();
      if (d) expect(formatYearMonth(d)).toBe(s);
    }
  });

  it('rejects malformed dates', () => {
    for (const s of ['1842-13', '1842-00', '18-01', '1842/01', 'Jan 1842', '1842-01-05', '1799']) {
      expect(parseYearMonth(s), s).toBeNull();
    }
  });
});
